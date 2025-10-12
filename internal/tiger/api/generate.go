package api

//go:generate go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen -config ../../../openapi-config.yaml -generate types -package api -o types.go ../../../openapi.yaml
//go:generate go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen -config ../../../openapi-config.yaml -generate client -package api -o client.go ../../../openapi.yaml
