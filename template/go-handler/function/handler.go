package function

import (
	"fmt"
	"net/http"

	handler "github.com/lavrahq/cli-function-go-sdk"
)

// Handle a function invocation
func Handle(req handler.Request, context handler.Context) (handler.Response, error) {
	var err error

	message := fmt.Sprintf("Hello world, input was: %s", string(req.Body))

	return handler.Response{
		Body:       []byte(message),
		StatusCode: http.StatusOK,
	}, err
}
