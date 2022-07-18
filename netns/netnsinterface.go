package netns

type NetnsInterface interface {
	Get() (fileDescriptor uintptr, err error)
	GetFromName(name string) (fileDescriptor uintptr, err error)
	Set(fileDescriptor uintptr) (err error)
	NewNamed(name string) (fileDescriptor uintptr, err error)
	DeleteNamed(name string) (err error)
}
