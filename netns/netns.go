package netns

import "github.com/vishvananda/netns"

type Netns struct{}

func NewNetns() *Netns {
	return &Netns{}
}

func (f *Netns) Get() (uintptr, error) {
	nsHandle, err := netns.Get()
	return uintptr(nsHandle), err
}
func (f *Netns) GetFromName(name string) (uintptr, error) {
	nsHandle, err := netns.GetFromName(name)
	return uintptr(nsHandle), err
}
func (f *Netns) Set(fileDescriptor uintptr) error {
	return netns.Set(netns.NsHandle(fileDescriptor))
}
func (f *Netns) NewNamed(name string) (uintptr, error) {
	nsHandle, err := netns.NewNamed(name)
	return uintptr(nsHandle), err
}
func (f *Netns) DeleteNamed(name string) error {
	return netns.DeleteNamed(name)
}
