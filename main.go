// Command notafter sends notifications via mail(1) if TLS certs will expire
// soon or have expired. The list of domains or subdomains is read from
// standard input, one per line.
package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const (
	expiryThreshold = 30 * 24 * time.Hour
	mailSubject     = "notafter: domain cert expiries"
)

func usage() {
	fmt.Fprintf(os.Stderr, "usage: notafter [<recipient>] < domains.txt\n")
}

func main() {
	log.SetPrefix("notafter: ")
	log.SetFlags(0)

	flag.Usage = usage
	flag.Parse()

	if flag.NArg() != 1 {
		usage()
		os.Exit(2)
	}

	recipient := flag.Arg(0)
	ctx := context.Background()
	now := time.Now()

	// parse domains.
	ds, err := domains(os.Stdin)
	if err != nil {
		log.Fatal(err)
	}
	if len(ds) == 0 {
		log.Fatal("no domains") // prevent common misconfiguration
	}

	items := make([]Item, len(ds))

	var wg sync.WaitGroup
	for i := range ds {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			end, err := check(ctx, ds[idx])
			items[idx] = Item{ds[idx], end, err}
		}(i)
	}
	wg.Wait()

	noNotify := func(i Item) bool { return !i.needsNotify(now) }
	if all(items, noNotify) {
		os.Exit(0)
	}

	if err := notify(items, now, recipient); err != nil {
		log.Fatal(err)
	}
}

func notify(items []Item, now time.Time, recipient string) error {
	return sendMail(recipient, mailBody(items, now))
}

func mailBody(items []Item, now time.Time) io.Reader {
	var buf bytes.Buffer
	for _, i := range items {
		buf.WriteString(i.format(now))
		buf.WriteByte('\n')
	}
	return &buf
}

func sendMail(recipient string, body io.Reader) error {
	cmd := exec.Command("mail", "-s", mailSubject, recipient)
	cmd.Stdin = body
	return cmd.Run()
}

type Item struct {
	domain string
	end    time.Time
	err    error // generic error
}

func (i Item) needsNotify(now time.Time) bool {
	if i.err != nil {
		return true
	}
	if i.end.Sub(now) > expiryThreshold {
		return false
	}
	return true
}

func (i Item) format(now time.Time) string {
	var w strings.Builder
	w.WriteString(i.domain + ": ")
	if i.err != nil {
		w.WriteString(i.err.Error())
	} else {
		w.WriteString(expiryInfo(i.end, now))
	}
	return w.String()
}

func expiryInfo(end, now time.Time) string {
	gap := end.Sub(now)
	switch {
	case gap > expiryThreshold:
		return "ok"
	case gap < 0:
		return "already expired"
	case gap < 24*time.Hour:
		return "expires in less than a day"
	default:
		n := gap / (24 * time.Hour)
		return fmt.Sprintf("expires in %d %s", n, pluralize(int64(n), "day"))
	}
}

func check(ctx context.Context, d string) (time.Time, error) {
	dialer := &tls.Dialer{
		Config: &tls.Config{
			InsecureSkipVerify: true,
		},
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	conn, err := dialer.DialContext(ctx, "tcp", fmt.Sprintf("%s:443", d))
	if err != nil {
		return time.Time{}, err
	}
	defer conn.Close()
	tlsConn := conn.(*tls.Conn) // guaranteed in package documentation

	cs := tlsConn.ConnectionState().PeerCertificates
	if len(cs) == 0 {
		return time.Time{}, errors.New("no peer certificates")
	}
	leaf := cs[0]
	return leaf.NotAfter, nil
}

func domains(r io.Reader) ([]string, error) {
	scanner := bufio.NewScanner(r)
	var out []string
	for scanner.Scan() {
		out = append(out, strings.TrimSpace(scanner.Text()))
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func pluralize(n int64, noun string) string {
	if n == 1 {
		return noun
	}
	return noun + "s"
}

func all[E any](s []E, f func(E) bool) bool {
	for _, v := range s {
		if !f(v) {
			return false
		}
	}
	return true
}
