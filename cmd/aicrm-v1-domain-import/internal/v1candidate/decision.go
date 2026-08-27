package v1candidate

type Disposition string

const (
	CanonicalCandidate  Disposition = "canonical_candidate"
	Quarantine          Disposition = "quarantine"
	Archive             Disposition = "archive"
	ReasonInvalidSource             = "invalid_source"
)

type Decision[T any] struct {
	Disposition Disposition
	Reason      string
	Candidate   *T
}

func canonical[T any](candidate T) Decision[T] {
	return Decision[T]{Disposition: CanonicalCandidate, Candidate: &candidate}
}

func quarantine[T any](reason string) Decision[T] {
	return Decision[T]{Disposition: Quarantine, Reason: reason}
}

func archive[T any](reason string) Decision[T] {
	return Decision[T]{Disposition: Archive, Reason: reason}
}
