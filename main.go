// Command my-cookies is a small CLI wrapper around github.com/browserutils/kooky.
//
// It takes a URL and prints the cookies stored by locally installed browsers
// that would be sent for that URL, similar to what browser_cookie3 does for
// Python.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/browserutils/kooky"
	_ "github.com/browserutils/kooky/browser/all" // register cookie store finders for every supported browser

	"github.com/spf13/pflag"
)

func main() {
	browserFlag := pflag.StringP("browser", "b", "", "only look at this browser (e.g. chrome, firefox)")
	all := pflag.BoolP("all", "a", false, "include expired cookies")
	jsonOut := pflag.BoolP("json", "j", false, "output JSON lines instead of a table")
	header := pflag.BoolP("header", "H", false, "output name=value pairs (\"; \" separated) usable as a Cookie header value")
	pflag.Parse()

	if pflag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: my-cookies [flags] <url>")
		pflag.PrintDefaults()
		os.Exit(2)
	}

	target := pflag.Arg(0)
	// allow bare hosts too, e.g. "example.com" instead of "https://example.com"
	if !strings.Contains(target, "://") {
		target = "https://" + target
	}
	u, err := url.Parse(target)
	if err != nil {
		log.Fatalf("invalid url %q: %v", pflag.Arg(0), err)
	}
	host := u.Hostname()
	if host == "" {
		log.Fatalf("could not determine host from %q", pflag.Arg(0))
	}

	filters := []kooky.Filter{
		kooky.FilterFunc(func(c *kooky.Cookie) bool {
			return c != nil && domainMatches(host, c.Domain) && pathMatches(u.Path, c.Path)
		}),
	}
	if !*all {
		filters = append(filters, kooky.Valid)
	}
	if *browserFlag != "" {
		b := *browserFlag
		filters = append(filters, kooky.FilterFunc(func(c *kooky.Cookie) bool {
			return c != nil && c.Browser != nil && strings.EqualFold(c.Browser.Browser(), b)
		}))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	seq := kooky.TraverseCookies(ctx, filters...)

	var cookies []*kooky.Cookie
	for cookie, err := range seq {
		if err != nil {
			continue
		}
		if cookie != nil {
			cookies = append(cookies, cookie)
		}
	}

	if len(cookies) == 0 {
		fmt.Fprintf(os.Stderr, "no cookies found for %s\n", host)
		os.Exit(1)
	}

	switch {
	case *header:
		printHeader(cookies)
	case *jsonOut:
		printJSON(cookies)
	default:
		printTable(cookies, host)
	}
}

// domainMatches reports whether a cookie set for cookieDomain would be sent
// to host, following the usual "domain or any subdomain" cookie matching
// rule (RFC 6265 §5.1.3).
func domainMatches(host, cookieDomain string) bool {
	d := strings.TrimPrefix(cookieDomain, ".")
	if strings.EqualFold(host, d) {
		return true
	}
	return len(host) > len(d) && strings.HasSuffix(strings.ToLower(host), "."+strings.ToLower(d))
}

func pathMatches(reqPath, cookiePath string) bool {
	if cookiePath == "" || cookiePath == "/" {
		return true
	}
	if reqPath == "" {
		reqPath = "/"
	}
	if reqPath == cookiePath {
		return true
	}
	return strings.HasPrefix(reqPath, strings.TrimSuffix(cookiePath, "/")+"/")
}

func printTable(cookies []*kooky.Cookie, host string) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "BROWSER\tPROFILE\tDOMAIN\tNAME\tVALUE\tEXPIRES")
	for _, c := range cookies {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			browserOf(c), profileOf(c), c.Domain, c.Name, c.Value, expiresOf(c))
	}
	w.Flush()
	fmt.Printf("\n%d cookie(s) matched for %s\n", len(cookies), host)
}

func printHeader(cookies []*kooky.Cookie) {
	seen := map[string]bool{}
	var parts []string
	for _, c := range cookies {
		if seen[c.Name] {
			continue
		}
		seen[c.Name] = true
		parts = append(parts, c.Name+"="+c.Value)
	}
	fmt.Println(strings.Join(parts, "; "))
}

func printJSON(cookies []*kooky.Cookie) {
	for _, c := range cookies {
		b, err := json.Marshal(c)
		if err != nil {
			continue
		}
		fmt.Println(string(b))
	}
}

func browserOf(c *kooky.Cookie) string {
	if c.Browser == nil {
		return ""
	}
	return c.Browser.Browser()
}

func profileOf(c *kooky.Cookie) string {
	if c.Browser == nil {
		return ""
	}
	return c.Browser.Profile()
}

func expiresOf(c *kooky.Cookie) string {
	if c.Expires.IsZero() {
		return "-"
	}
	return c.Expires.Format("2006-01-02 15:04:05")
}
