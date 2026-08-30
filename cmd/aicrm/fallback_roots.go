package main

// The release image is intentionally scratch-only, so provider HTTPS needs a
// compiled fallback root set when no operating-system certificate pool exists.
import _ "golang.org/x/crypto/x509roots/fallback"
