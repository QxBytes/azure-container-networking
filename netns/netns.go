package netns

import "github.com/vishvananda/netns"

type Netns struct{}

func New() *Netns {
	return &Netns{}
}

func (f *Netns) Get() (int, error) {
	nsHandle, err := netns.Get()
	return int(nsHandle), err
}
func (f *Netns) GetFromName(name string) (int, error) {
	nsHandle, err := netns.GetFromName(name)
	return int(nsHandle), err
}
func (f *Netns) Set(fileDescriptor int) error {
	return netns.Set(netns.NsHandle(fileDescriptor))
}
func (f *Netns) NewNamed(name string) (int, error) {
	nsHandle, err := netns.NewNamed(name)
	return int(nsHandle), err
}
func (f *Netns) DeleteNamed(name string) error {
	return netns.DeleteNamed(name)
}
