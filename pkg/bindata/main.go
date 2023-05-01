package main

import (
	v3bindata "github.com/go-bindata/go-bindata/v3"
)

const CRDPath = "./chart/seeder-crd/templates/"

func main() {

	c := &v3bindata.Config{
		Input: []v3bindata.InputConfig{{
			Path: CRDPath,
		}},
		Output:  "./pkg/data/data.go",
		Package: "data",
	}

	err := v3bindata.Translate(c)
	if err != nil {
		panic(err)
	}
}
