package docs

import (
	_ "embed"
)

var (
	//go:embed *swagger.json
	swagger string
)

//get docs
func GetDocs() string {
	return swagger
}
