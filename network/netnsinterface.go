package network

type NetnsInterface interface {
	Get() (fileDescriptor int, err error)
	GetFromName(name string) (fileDescriptor int, err error)
	Set(fileDescriptor int) (err error)
	NewNamed(name string) (fileDescriptor int, err error)
	DeleteNamed(name string) (err error)
}
