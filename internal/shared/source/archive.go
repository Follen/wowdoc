package source

type ArchiveFetcher interface {
	FetchArchive(repoURL, ref, destination string) error
}
