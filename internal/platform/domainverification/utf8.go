package domainverification

import "unicode/utf8"

func utf8Valid(content []byte) bool { return utf8.Valid(content) }
