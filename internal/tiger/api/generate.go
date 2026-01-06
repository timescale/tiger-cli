package api

//go:generate go tool oapi-codegen -generate types -package api -o types.go ../../../openapi.yaml
//go:generate go tool oapi-codegen -generate client -package api -o client.go ../../../openapi.yaml
