package template

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jmespath/go-jmespath"
)

// QueryObject applies a JMESPath query specified by the expression, against the target object.
func QueryObject(exp string, target interface{}) (interface{}, error) {
	query, err := jmespath.Compile(exp)
	if err != nil {
		return nil, err
	}
	return query.Search(target)
}

// SplitLines splits the input into a string slice.
func SplitLines(o interface{}) ([]string, error) {
	ret := []string{}
	switch o := o.(type) {
	case string:
		return strings.Split(o, "\n"), nil
	case []byte:
		return strings.Split(string(o), "\n"), nil
	}
	return ret, fmt.Errorf("not-supported-value-type")
}

// FromJSON decode the input JSON encoded as string or byte slice into a map.
func FromJSON(o interface{}) (interface{}, error) {
	ret := map[string]interface{}{}
	switch o := o.(type) {
	case string:
		err := json.Unmarshal([]byte(o), &ret)
		return ret, err
	case []byte:
		err := json.Unmarshal(o, &ret)
		return ret, err
	}
	return ret, fmt.Errorf("not-supported-value-type")
}

// ToJSON encodes the input struct into a JSON string.
func ToJSON(o interface{}) (string, error) {
	buff, err := json.MarshalIndent(o, "", "  ")
	return string(buff), err
}

// FromMap decodes map into raw struct
func FromMap(m map[string]interface{}, raw interface{}) error {
	// The safest way, but the slowest, is to just marshal and unmarshal back
	buff, err := ToJSON(m)
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(buff), raw)
}

// ToMap encodes the input as a map
func ToMap(raw interface{}) (map[string]interface{}, error) {
	buff, err := ToJSON(raw)
	if err != nil {
		return nil, err
	}
	out, err := FromJSON(buff)
	return out.(map[string]interface{}), err
}

// UnixTime returns a timestamp in unix time
func UnixTime() interface{} {
	return time.Now().Unix()
}

// DefaultFuncs returns a list of default functions for binding in the template
func (t *Template) DefaultFuncs() map[string]interface{} {
	return map[string]interface{}{
		"include": func(p string, opt ...interface{}) (string, error) {
			var o interface{}
			if len(opt) > 0 {
				o = opt[0]
			}
			loc, err := getURL(t.url, p)
			if err != nil {
				return "", err
			}
			included, err := NewTemplate(loc, t.options)
			if err != nil {
				return "", err
			}
			// copy the binds in the parent scope into the child
			for k, v := range t.binds {
				included.binds[k] = v
			}
			// inherit the functions defined for this template
			for k, v := range t.funcs {
				included.AddFunc(k, v)
			}
			return included.Render(o)
		},

		"var": func(name, doc string, v ...interface{}) interface{} {
			if found, has := t.binds[name]; has {
				return found
			}
			return v // default
		},

		"global": func(name string, v interface{}) interface{} {
			t.binds[name] = v
			return ""
		},

		"q":         QueryObject,
		"unixtime":  UnixTime,
		"lines":     SplitLines,
		"to_json":   ToJSON,
		"from_json": FromJSON,
	}
}
