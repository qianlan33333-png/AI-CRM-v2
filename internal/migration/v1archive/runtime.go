package v1archive

// Runtime contains the operator-supplied CLI configuration. Keeping the
// importer independently from app composition.
type Runtime struct {
	SourceDatabaseURL string
	TargetDatabaseURL string
	SourceHMACKey     string
	ArchiveKey        string
}
