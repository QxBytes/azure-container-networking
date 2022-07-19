package netns

import (
	"github.com/pkg/errors"
)

const (
	Get         int = 1
	GetFromName int = 2
	Set         int = 3
	NewNamed    int = 4
	DeleteNamed int = 5
)

var ErrorMock = errors.New("mock netns error")

func newErrorMock(errStr string) error {
	return errors.Wrap(ErrorMock, errStr)
}

type MockNetns struct {
	failMethod  int
	failMessage string
}

func NewMock(failMethod int, failMessage string) *MockNetns {
	return &MockNetns{
		failMethod:  failMethod,
		failMessage: failMessage,
	}
}

func (f *MockNetns) Get() (int, error) {
	if f.failMethod == Get {
		return 0, newErrorMock(f.failMessage)
	}
	return 1, nil
}
func (f *MockNetns) GetFromName(name string) (int, error) {
	if f.failMethod == GetFromName {
		return 0, newErrorMock(f.failMessage)
	}
	return 1, nil
}
func (f *MockNetns) Set(handle int) error {
	if f.failMethod == Set {
		return newErrorMock(f.failMessage)
	}
	return nil
}
func (f *MockNetns) NewNamed(name string) (int, error) {
	if f.failMethod == NewNamed {
		return 0, newErrorMock(f.failMessage)
	}
	return 1, nil
}
func (f *MockNetns) DeleteNamed(name string) error {
	if f.failMethod == DeleteNamed {
		return newErrorMock(f.failMessage)
	}
	return nil
}
