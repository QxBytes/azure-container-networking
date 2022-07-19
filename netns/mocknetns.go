package netns

import (
	"github.com/pkg/errors"
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
	if f.failMethod == 1 {
		return 0, newErrorMock(f.failMessage)
	}
	return 1, nil
}
func (f *MockNetns) GetFromName(name string) (int, error) {
	if f.failMethod == 2 {
		return 0, newErrorMock(f.failMessage)
	}
	return 1, nil
}
func (f *MockNetns) Set(handle int) error {
	if f.failMethod == 3 {
		return newErrorMock(f.failMessage)
	}
	return nil
}
func (f *MockNetns) NewNamed(name string) (int, error) {
	if f.failMethod == 4 {
		return 0, newErrorMock(f.failMessage)
	}
	return 1, nil
}
func (f *MockNetns) DeleteNamed(name string) error {
	if f.failMethod == 5 {
		return newErrorMock(f.failMessage)
	}
	return nil
}
