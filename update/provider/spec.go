package provider

type Interface interface {
	CurrentVersion() (string, error)
	NextVersion() (string, error)
	UpdateVersion(nextVersion string) error
	WaitForUpdate(nextVersion string) error
}

type Patch struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value"`
}
