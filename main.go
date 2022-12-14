// Command notafter sends notifications via mail(1) if TLS certs for the
// specified domains will expire soon or have expired. The list of domains is
// read from standard input, one per line.
//
// The program exits with a non-zero exit status upon internal errors (e.g.
// failure to invoke mail(1)). On the other hand, any failures to reach
// specified domains do not result in a non-zero exit status; such errors are
// mailed instead, and the command will exit with a zero status.
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
	notifyExpiryThreshold = 28 * 24 * time.Hour
	mailSubject           = "notafter: domain cert expiries"
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
			end, err := getCertEnd(ctx, ds[idx])
			items[idx] = Item{ds[idx], end, err}
		}(i)
	}
	wg.Wait()

	noNotify := func(i Item) bool { return !i.needsNotify(now) }
	if all(items, noNotify) {
		os.Exit(0)
	}

	body := resultsBody(items, now)

	// print results to stdout.
	fmt.Print(body)

	// mail the results.
	err = sendMail(recipient, body)
	if err != nil {
		log.Fatal(err)
	}
}

func resultsBody(items []Item, now time.Time) string {
	var buf bytes.Buffer
	for _, i := range items {
		buf.WriteString(i.format(now))
		buf.WriteByte('\n')
	}
	return buf.String()
}

func sendMail(recipient string, body string) error {
	cmd := exec.Command("mail", "-s", mailSubject, recipient)
	cmd.Stdin = strings.NewReader(body)
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
	if i.end.Sub(now) > notifyExpiryThreshold {
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
	case gap > notifyExpiryThreshold:
		return "good"
	case gap < 0:
		return "expired"
	case gap < 24*time.Hour:
		return "expires in less than 24h"
	default:
		n := gap / (24 * time.Hour)
		return fmt.Sprintf("expires in %d %s", n, pluralize(int64(n), "day"))
	}
}

func getCertEnd(ctx context.Context, domain string) (time.Time, error) {
	dialer := &tls.Dialer{
		Config: &tls.Config{
			InsecureSkipVerify: true,
		},
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	conn, err := dialer.DialContext(ctx, "tcp", fmt.Sprintf("%s:443", domain))
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
	return out, scanner.Err()
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
