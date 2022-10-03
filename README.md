Command notafter sends notifications via mail(1) if TLS certs for the
specified domains will expire soon or have expired. The list of domains is
read from standard input, one per line.

The command is called "notafter" because it works on the "notAfter" date on
certificates.

## Install

With Go 1.19 or higher:

```
go install github.com/nishanths/notafter
```

## Usage

See `notafter -h`.

You may want to run the command regularly as a part of a cron or a recursive
at(1) job.
