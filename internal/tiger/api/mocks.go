package api

//go:generate go tool mockgen -source=client.go -destination=mocks/mock_client.go -package=mocks ClientWithResponsesInterface
