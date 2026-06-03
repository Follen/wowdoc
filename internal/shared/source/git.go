package source

type GitRunner interface {
	Run(args ...string) error
	Output(args ...string) ([]byte, error)
}
