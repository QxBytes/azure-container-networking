package netns

import (
	"errors"
	"fmt"
)

var ErrorMockNetns = errors.New("mock netns error")

func newErrorMockNetns(errStr string) error {
	return fmt.Errorf("%w : %s", ErrorMockNetns, errStr)
}

type MockNetns struct {
	failMethod  int
	failMessage string
}

func NewMockNetns(failMethod int, failMessage string) *MockNetns {
	return &MockNetns{
		failMethod:  failMethod,
		failMessage: failMessage,
	}
}

func (f *MockNetns) Get() (uintptr, error) {
	if f.failMethod == 1 {
		return 0, newErrorMockNetns(f.failMessage)
	}
	return 1, nil
}
func (f *MockNetns) GetFromName(name string) (uintptr, error) {
	if f.failMethod == 2 {
		return 0, newErrorMockNetns(f.failMessage)
	}
	return 1, nil
}
func (f *MockNetns) Set(handle uintptr) error {
	if f.failMethod == 3 {
		return newErrorMockNetns(f.failMessage)
	}
	return nil
}
func (f *MockNetns) NewNamed(name string) (uintptr, error) {
	if f.failMethod == 4 {
		return 0, newErrorMockNetns(f.failMessage)
	}
	return 1, nil
}
func (f *MockNetns) DeleteNamed(name string) error {
	if f.failMethod == 5 {
		return newErrorMockNetns(f.failMessage)
	}
	return nil
}
