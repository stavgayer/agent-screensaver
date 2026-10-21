package platform

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestAnswerPage(t *testing.T) {
	echo := func(method string, params json.RawMessage) (any, error) {
		switch method {
		case "echo":
			return params, nil
		case "fail":
			return nil, errors.New(`no "theme" here`)
		case "unencodable":
			return func() {}, nil
		}
		return nil, nil
	}
	tests := []struct {
		name    string
		message string
		want    string
		wantErr bool
	}{
		{
			name:    "result goes back with the id",
			message: `{"id":7,"method":"echo","params":{"id":"builtin.default"}}`,
			want:    `window.__agsReply(7, {"id":"builtin.default"}, null)`,
		},
		{
			name:    "error goes back as a string",
			message: `{"id":8,"method":"fail"}`,
			want:    `window.__agsReply(8, null, "no \"theme\" here")`,
		},
		{
			name:    "nil result is null",
			message: `{"id":9,"method":"nothing"}`,
			want:    `window.__agsReply(9, null, null)`,
		},
		{
			name:    "text that ends a script or a line in JavaScript is escaped",
			message: "{\"id\":10,\"method\":\"echo\",\"params\":\"</script>\u2028\"}",
			want:    "window.__agsReply(10, \"\\u003c/script\\u003e\\u2028\", null)",
		},
		{
			name:    "a result that cannot be encoded becomes an error",
			message: `{"id":11,"method":"unencodable"}`,
			want:    `window.__agsReply(11, null, "encode result: json: unsupported type: func()")`,
		},
		{
			name:    "a message that is not a request is refused",
			message: `not json`,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := answerPage([]byte(tt.message), echo)
			if (err != nil) != tt.wantErr {
				t.Fatalf("answerPage error = %v, want error %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("answerPage =\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}
