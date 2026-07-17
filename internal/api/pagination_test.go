package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPageParamsBoundsInput(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/?limit=500&offset=-2", nil)
	limit, offset := pageParams(req, 25)
	if limit != 100 || offset != 0 {
		t.Fatalf("pageParams = (%d, %d), want (100, 0)", limit, offset)
	}
}

func TestRespondPageUsesSentinelRowForHasMore(t *testing.T) {
	res := httptest.NewRecorder()
	respondPage(res, []int{1, 2, 3}, 2, 4)

	var body struct {
		Data []int    `json:"data"`
		Meta pageMeta `json:"meta"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Data) != 2 || !body.Meta.HasMore || body.Meta.Limit != 2 || body.Meta.Offset != 4 {
		t.Fatalf("page response = %#v", body)
	}
}
