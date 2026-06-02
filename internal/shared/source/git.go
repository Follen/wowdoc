package source

type GitRunner interface {
	Run(args ...string) error
}
