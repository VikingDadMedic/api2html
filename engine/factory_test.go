package engine

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestFactory_New_koConfigParser(t *testing.T) {
	expectedErr := fmt.Errorf("boooom")
	ef := Factory{
		Parser: func(path string) (Config, error) {
			if path != "something" {
				t.Errorf("unexpected path: %s", path)
			}
			return Config{}, expectedErr
		},
	}
	if _, err := ef.New("something", true); err == nil {
		t.Error("expecting error")
	} else if err != expectedErr {
		t.Errorf("unexpected error: %s", err.Error())
	}
}

func TestFactory_New_ok(t *testing.T) {
	if err := ioutil.WriteFile("test_tmpl", []byte("hi, {{Extra.name}}!"), 0644); err != nil {
		t.Errorf("unexpected error: %s", err.Error())
	}
	if err := ioutil.WriteFile("test_lyt", []byte("-{{{content}}}-"), 0644); err != nil {
		t.Errorf("unexpected error: %s", err.Error())
	}
	defer os.Remove("test_tmpl")
	defer os.Remove("test_lyt")
	expectedCfg := Config{
		Pages: []Page{
			{
				URLPattern: "/a",
				Layout:     "b",
				Template:   "a",
				Extra: map[string]interface{}{
					"name": "stranger",
				},
			},
		},
		Templates: map[string]string{"a": "test_tmpl"},
		Layouts:   map[string]string{"b": "test_lyt"},
	}
	templateStore := NewTemplateStore()
	ef := DefaultFactory
	ef.Parser = func(path string) (Config, error) {
		if path != "something" {
			t.Errorf("unexpected path: %s", path)
		}
		return expectedCfg, nil
	}
	ef.TemplateStoreFactory = func() *TemplateStore { return templateStore }
	ef.MustachePageFactory = func(e *gin.Engine, ts *TemplateStore) MustachePageFactory {
		if ts != templateStore {
			t.Errorf("unexpected template store: %v", ts)
		}
		return NewMustachePageFactory(e, ts)
	}

	e, err := ef.New("something", true)
	if err != nil {
		t.Errorf("unexpected error: %s", err.Error())
		return
	}

	time.Sleep(200 * time.Millisecond)

	assertResponse(t, e, "/a", http.StatusOK, "-hi, stranger!-")
	assertResponse(t, e, "/b", http.StatusNotFound, default404Tmpl)
}

func TestFactory_New_reloadTemplate(t *testing.T) {
	if err := ioutil.WriteFile("test_tmpl", []byte("hi, {{Extra.name}}!"), 0644); err != nil {
		t.Errorf("unexpected error: %s", err.Error())
	}
	defer os.Remove("test_tmpl")

	expectedCfg := Config{
		Pages: []Page{
			{
				URLPattern: "/a",
				Template:   "a",
				Extra: map[string]interface{}{
					"name": "stranger",
				},
			},
		},
		Templates: map[string]string{"a": "test_tmpl"},
	}
	templateStore := NewTemplateStore()
	ef := DefaultFactory
	ef.Parser = func(path string) (Config, error) {
		if path != "something" {
			t.Errorf("unexpected path: %s", path)
		}
		return expectedCfg, nil
	}
	ef.TemplateStoreFactory = func() *TemplateStore { return templateStore }
	ef.MustachePageFactory = func(e *gin.Engine, ts *TemplateStore) MustachePageFactory {
		if ts != templateStore {
			t.Errorf("unexpected template store: %v", ts)
		}
		return NewMustachePageFactory(e, ts)
	}

	e, err := ef.New("something", true)
	if err != nil {
		t.Errorf("unexpected error: %s", err.Error())
		return
	}

	time.Sleep(200 * time.Millisecond)

	// Non-existent file param
	req, _ := http.NewRequest("PUT", "/template/a", nil)
	resp := httptest.NewRecorder()
	e.ServeHTTP(resp, req)

	// Invalid template
	req, err = putTemplateForm("/template/a", "Hi {{ I'm template with errors.")
	if err != nil {
		t.Errorf("Error creating PUT Form body: %s", err.Error())
	}
	resp = httptest.NewRecorder()
	e.ServeHTTP(resp, req)

	if statusCode := resp.Result().StatusCode; statusCode != http.StatusInternalServerError {
		t.Errorf("[%s] unexpected status code: %d (%v)", "/template/a", statusCode, resp.Result())
	}

	// Hot reload correctly a template
	req, err = putTemplateForm("/template/a", "Hi {{Extra.name}}, I'm updated.")
	if err != nil {
		t.Errorf("Error creating PUT Form body: %s", err.Error())
	}
	resp = httptest.NewRecorder()
	e.ServeHTTP(resp, req)
	time.Sleep(200 * time.Millisecond)
	assertResponse(t, e, "/a", http.StatusOK, "Hi stranger, I'm updated.")

}

func putTemplateForm(url, tmpl string) (*http.Request, error) {
	buff := &bytes.Buffer{}
	tmplWriter := multipart.NewWriter(buff)
	fileWriter, err := tmplWriter.CreateFormFile("file", "test_tmpl")
	if err != nil {
		tmplWriter.Close()
		return nil, err
	}

	_, err = io.WriteString(fileWriter, tmpl)
	tmplWriter.Close()
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("PUT", url, buff)
	if err == nil {
		req.Header.Set("Content-Type", tmplWriter.FormDataContentType())
	}
	return req, err
}
