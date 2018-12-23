package mwerr

import (
	"net/http"
)

var _ Public = &EtfErr{}

type EtfErr struct {
	E error
}

func (e *EtfErr) Error() string {
	return e.E.Error()
}

func (e *EtfErr) Public() (string, int) {
	return e.Error(), http.StatusInternalServerError
}
