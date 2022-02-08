package hapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
)

type (
	Proc  func(h *Handler) (int, interface{})
	Param struct { //parameters in G-P-C order
		Name     string `json:"name"`
		Type     string `json:"type"` //string, int, float, bool, file
		Default  string `json:"default"`
		Required bool   `json:"required"`
		Memo     string `json:"memo"`
		defval   interface{}
	}
	Handler struct {
		spec  []Param
		opts  map[string]interface{} //url parameters
		args  []string               //path arguments
		req   *http.Request          //raw request
		err   error                  //argument processing error
		Route string
		hdr   http.Header
		proc  Proc
	}
)

var defaultMIME string

func (h *Handler) Error() error {
	return h.err
}

func (h *Handler) Header(k, v string) {
	if v == "" {
		h.hdr.Del(k)
	} else {
		h.hdr.Set(k, v)
	}
}

func args(r *http.Request) (url.Values, error) {
	vs := make(url.Values)
	for _, c := range r.Cookies() {
		vs[c.Name] = []string{c.Value}
	}
	switch r.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch:
		ct, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
		switch ct {
		case "application/json":
			var kv map[string]interface{}
			err := json.NewDecoder(r.Body).Decode(&kv)
			if err != nil {
				return nil, err
			}
			for k, v := range kv {
				switch v := v.(type) {
				case string:
					vs[k] = []string{v}
				case []string:
					vs[k] = v
				default:
					vs[k] = []string{fmt.Sprintf("%v", v)}
				}
			}
		case "multipart/form-data":
			err := r.ParseMultipartForm(10 << 20)
			if err != nil {
				return nil, err
			}
			fallthrough
		default:
			err := r.ParseForm()
			if err != nil {
				return nil, err
			}
			for k, v := range r.Form {
				vs[k] = v
			}
		}
	}
	for k, v := range r.URL.Query() {
		vs[k] = v
	}
	pq, _ := url.ParseQuery(r.URL.Path[1:])
	for k, v := range pq {
		k = path.Base(k)
		if !vs.Has(k) {
			vs[k] = v
		}
	}
	return vs, nil
}

func (h *Handler) parseArgs(r *http.Request) {
	h.req = r
	if sfx := r.URL.Path[len(h.Route):]; len(sfx) > 0 {
		h.args = strings.Split(sfx, "/")
	}
	vs, err := args(r)
	if err != nil {
		h.err = err
		return
	}
	for _, p := range h.spec {
		v := vs[p.Name]
		if len(v) == 0 && p.Required {
			h.err = fmt.Errorf("missing %q", p.Name)
			return
		}
		switch p.Type {
		case "string":
			if len(v) == 0 {
				h.opts[p.Name] = []string{p.defval.(string)}
			} else {
				h.opts[p.Name] = v
			}
		case "int":
			var is []int64
			for _, s := range v {
				i, err := strconv.Atoi(s)
				if err != nil {
					h.err = fmt.Errorf("%q is not an integer (arg:%s)", s, p.Name)
					return
				}
				is = append(is, int64(i))
			}
			if len(is) == 0 {
				is = []int64{p.defval.(int64)}
			}
			h.opts[p.Name] = is
		case "float":
			var fs []float64
			for _, s := range v {
				f, err := strconv.ParseFloat(s, 64)
				if err != nil {
					h.err = fmt.Errorf("%q is not a float (arg:%s)", s, p.Name)
					return
				}
				fs = append(fs, f)
			}
			if len(fs) == 0 {
				fs = []float64{p.defval.(float64)}
			}
			h.opts[p.Name] = fs
		case "bool":
			var bs []bool
			for _, s := range v {
				b := true
				if s != "" {
					b, err = strconv.ParseBool(s)
					if err != nil {
						h.err = fmt.Errorf("%q is not a bool (arg:%s)", s, p.Name)
						return
					}
				}
				bs = append(bs, b)
			}
			if len(bs) == 0 {
				bs = []bool{p.defval.(bool)}
			}
			h.opts[p.Name] = bs
		}
	}
}

func (h *Handler) Strings(name string) ([]string, error) {
	switch v := h.opts[name].(type) {
	case nil:
		return nil, fmt.Errorf("parameter %q does not exist", name)
	case []string:
		return v, nil
	default:
		return nil, fmt.Errorf("parameter %q is %T, not string", name, v)
	}
}

func (h *Handler) String(name string) (string, error) {
	ss, err := h.Strings(name)
	if err != nil {
		return "", err
	}
	return ss[0], nil
}

func (h *Handler) Integers(name string) ([]int64, error) {
	switch v := h.opts[name].(type) {
	case nil:
		return nil, fmt.Errorf("parameter %q does not exist", name)
	case []int64:
		return v, nil
	default:
		return nil, fmt.Errorf("parameter %q is %T, not integer", name, v)
	}
}

func (h *Handler) Integer(name string) (int64, error) {
	is, err := h.Integers(name)
	if err != nil {
		return 0, err
	}
	return is[0], nil
}

func (h *Handler) Floats(name string) ([]float64, error) {
	switch v := h.opts[name].(type) {
	case nil:
		return nil, fmt.Errorf("parameter %q does not exist", name)
	case []float64:
		return v, nil
	default:
		return nil, fmt.Errorf("parameter %q is %T, not float", name, v)
	}
}

func (h *Handler) Float(name string) (float64, error) {
	fs, err := h.Floats(name)
	if err != nil {
		return 0, err
	}
	return fs[0], nil
}

func (h *Handler) Bools(name string) ([]bool, error) {
	switch v := h.opts[name].(type) {
	case nil:
		return nil, fmt.Errorf("parameter %q does not exist", name)
	case []bool:
		return v, nil
	default:
		return nil, fmt.Errorf("parameter %q is %T, not bool", name, v)
	}
}

func (h *Handler) Bool(name string) (bool, error) {
	bs, err := h.Bools(name)
	if err != nil {
		return false, err
	}
	return bs[0], nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.proc == nil {
		http.Error(w, http.StatusText(http.StatusNotImplemented), http.StatusNotImplemented)
		return
	}
	h.parseArgs(r)
	code, data := h.proc(h)
	if h.hdr.Get("Content-Type") == "" {
		h.hdr.Set("Content-Type", defaultMIME)
	}
	for k, vs := range h.hdr {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(code)
	var err error
	switch d := data.(type) {
	case []byte:
		_, err = w.Write(d)
	case string:
		_, err = w.Write([]byte(d))
	case io.Reader:
		_, err = io.Copy(w, d)
	default:
		err = errors.New("data type must be []byte, string or io.Reader")
	}
	if err != nil {
		panic(err)
	}
}

func NewHandler(route string, spec []Param, proc Proc) (h *Handler, err error) {
	for i, p := range spec {
		t := strings.ToLower(p.Type)
		var v interface{}
		switch t {
		case "string":
			v = p.Default
		case "int":
			i := int(0)
			if p.Default != "" {
				i, err = strconv.Atoi(p.Default)
				if err != nil {
					return nil, fmt.Errorf("default value %q is not a valid integer", p.Default)
				}
			}
			v = int64(i)
		case "float":
			f := float64(0)
			if p.Default != "" {
				f, err = strconv.ParseFloat(p.Default, 64)
				if err != nil {
					return nil, fmt.Errorf("default value %q is not a valid float", p.Default)
				}
			}
			v = f
		case "bool":
			b := false
			if p.Default != "" {
				b, err = strconv.ParseBool(p.Default)
				if err != nil {
					return nil, fmt.Errorf("default value %q is not a valid bool", p.Default)
				}
			}
			v = b
		default:
			return nil, fmt.Errorf("invalid param type %q", p.Type)
		}
		spec[i].Type = t
		spec[i].defval = v
	}
	h = &Handler{
		spec:  spec,
		Route: route,
		hdr:   make(http.Header),
		proc:  proc,
	}
	return
}

func MIMEType(mime ...string) string {
	if len(mime) > 0 {
		defaultMIME = mime[0]
	}
	return defaultMIME
}

func init() {
	defaultMIME = "text/plain; charset=utf-8"
}
