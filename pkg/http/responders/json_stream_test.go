package responders_test

import (
	"context"
	"encoding/json"
	goerrors "errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/TriangleSide/GoBase/pkg/http/errors"
	"github.com/TriangleSide/GoBase/pkg/http/headers"
	"github.com/TriangleSide/GoBase/pkg/http/responders"
	"github.com/TriangleSide/GoBase/pkg/test/assert"
)

func TestJSONStreamResponder(t *testing.T) {
	t.Parallel()

	type requestParams struct {
		ID int `json:"id" validate:"gt=0"`
	}

	type responseBody struct {
		Message string `json:"message"`
	}

	t.Run("when the callback function processes the request successfully it should respond with the correct JSON stream response and status code", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			responders.JSONStream[requestParams, responseBody](w, r, func(params *requestParams) (<-chan *responseBody, int, error) {
				ch := make(chan *responseBody)
				go func() {
					defer close(ch)
					ch <- &responseBody{Message: "first"}
					ch <- &responseBody{Message: "second"}
				}()
				return ch, http.StatusOK, nil
			})
		}))
		defer server.Close()

		response, err := http.Post(server.URL, headers.ContentTypeApplicationJson, strings.NewReader(`{"id":1}`))
		assert.NoError(t, err)
		assert.Equals(t, response.StatusCode, http.StatusOK)

		decoder := json.NewDecoder(response.Body)
		responseObj := &responseBody{}
		assert.NoError(t, decoder.Decode(responseObj))
		assert.Equals(t, responseObj.Message, "first")
		assert.NoError(t, decoder.Decode(responseObj))
		assert.Equals(t, responseObj.Message, "second")
		assert.NoError(t, response.Body.Close())
	})

	t.Run("when the parameter decoder fails it should respond with an error JSON response and appropriate status code", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			responders.JSONStream[requestParams, responseBody](w, r, func(params *requestParams) (<-chan *responseBody, int, error) {
				return nil, 0, &errors.BadRequest{Err: goerrors.New("invalid parameters")}
			})
		}))
		defer server.Close()

		response, err := http.Post(server.URL, headers.ContentTypeApplicationJson, strings.NewReader(`{"id":-1}`))
		assert.NoError(t, err)
		assert.Equals(t, response.StatusCode, http.StatusBadRequest)

		responseObj := &errors.Error{}
		assert.NoError(t, json.NewDecoder(response.Body).Decode(responseObj))
		assert.Contains(t, responseObj.Message, "validation failed on field 'ID'")
		assert.NoError(t, response.Body.Close())
	})

	t.Run("when the callback function returns an error it should respond with an error JSON response and appropriate status code", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			responders.JSONStream[requestParams, responseBody](w, r, func(params *requestParams) (<-chan *responseBody, int, error) {
				return nil, 0, &errors.BadRequest{Err: goerrors.New("invalid parameters")}
			})
		}))
		defer server.Close()

		response, err := http.Post(server.URL, headers.ContentTypeApplicationJson, strings.NewReader(`{"id":2}`))
		assert.NoError(t, err)
		assert.Equals(t, response.StatusCode, http.StatusBadRequest)

		responseObj := &errors.Error{}
		assert.NoError(t, json.NewDecoder(response.Body).Decode(responseObj))
		assert.Equals(t, responseObj.Message, "invalid parameters")
		assert.NoError(t, response.Body.Close())
	})

	t.Run("when the callback function returns a response that cannot be encoded it should not write the body", func(t *testing.T) {
		t.Parallel()

		type unmarshalableResponse struct {
			ChanField chan int `json:"chan_field"`
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			responders.JSONStream[requestParams, unmarshalableResponse](w, r, func(params *requestParams) (<-chan *unmarshalableResponse, int, error) {
				ch := make(chan *unmarshalableResponse, 1)
				go func() {
					defer close(ch)
					ch <- &unmarshalableResponse{}
				}()
				return ch, http.StatusOK, nil
			})
		}))
		defer server.Close()

		response, err := http.Post(server.URL, headers.ContentTypeApplicationJson, strings.NewReader(`{"id":3}`))
		assert.NoError(t, err)
		assert.Equals(t, response.StatusCode, http.StatusOK)

		body := make(map[string]interface{})
		err = json.NewDecoder(response.Body).Decode(&body)
		assert.Error(t, err)
		assert.NoError(t, response.Body.Close())
	})

	t.Run("when the request context is cancelled when streaming json it should not write any data to the response body", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithCancel(r.Context())
			r = r.WithContext(ctx)
			cancel()
			responders.JSONStream[requestParams, responseBody](w, r, func(params *requestParams) (<-chan *responseBody, int, error) {
				ch := make(chan *responseBody)
				go func() {
					defer close(ch)
					ch <- &responseBody{Message: "first"}
				}()
				return ch, http.StatusOK, nil
			})
		}))
		defer server.Close()

		response, err := http.Post(server.URL, headers.ContentTypeApplicationJson, strings.NewReader(`{"id":4}`))
		assert.NoError(t, err)
		assert.Equals(t, response.StatusCode, http.StatusOK)

		body := make(map[string]interface{})
		err = json.NewDecoder(response.Body).Decode(&body)
		assert.ErrorPart(t, err, "EOF")
		assert.NoError(t, response.Body.Close())
	})
}
