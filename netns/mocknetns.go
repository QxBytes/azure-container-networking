//go:build linux
// +build linux

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
	failMethod2 int
	failMessage string
}

func NewMock(failMethod, failMethod2 int, failMessage string) *MockNetns {
	return &MockNetns{
		failMethod:  failMethod,
		failMethod2: failMethod2,
		failMessage: failMessage,
	}
}

func (f *MockNetns) Get() (int, error) {
	if f.failMethod == Get || f.failMethod2 == Get {
		return 0, newErrorMock(f.failMessage)
	}
	return 1, nil
}

func (f *MockNetns) GetFromName(name string) (int, error) {
	if f.failMethod == GetFromName || f.failMethod2 == GetFromName {
		return 0, newErrorMock(f.failMessage)
	}
	return 1, nil
}

func (f *MockNetns) Set(handle int) error {
	if f.failMethod == Set || f.failMethod2 == Set {
		return newErrorMock(f.failMessage)
	}
	return nil
}

func (f *MockNetns) NewNamed(name string) (int, error) {
	if f.failMethod == NewNamed || f.failMethod2 == NewNamed {
		return 0, newErrorMock(f.failMessage)
	}
	return 1, nil
}

func (f *MockNetns) DeleteNamed(name string) error {
	if f.failMethod == DeleteNamed || f.failMethod2 == DeleteNamed {
		return newErrorMock(f.failMessage)
	}
	return nil
}
