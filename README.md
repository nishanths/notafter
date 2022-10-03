Command notafter sends notifications via mail(1) if TLS certs for the
specified domains will expire soon or have expired.  For more details, see the
package [documentation][1].

The command is called "notafter" because it works on the "notAfter" date on
certificates.

## Install

With Go 1.19 or higher:

```
go install github.com/nishanths/notafter
```

## Usage

See `notafter -h`. You may want to run the command regularly as a part of a
cron or a recursive at(1) job.

[1]: https://pkg.go.dev/github.com/nishantsh/notafter
