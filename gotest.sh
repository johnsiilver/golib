#! /bin/sh
go test `go list ./... | grep -v development`
