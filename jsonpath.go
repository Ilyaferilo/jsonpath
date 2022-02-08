package jsonpath

import (
	"errors"
	"fmt"
	"go/token"
	"go/types"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

const (
	KeyOp    = "key"
	IndexOp  = "idx"
	RangeOp  = "range"
	FilterOp = "filter"
)

var ErrGetFromNullObj = errors.New("get attribute from null object")

// NotExist is returned by jsonpath.Get on nonexistent paths
type NotExist struct {
	key string
}

func (d NotExist) Error() string {
	return fmt.Sprintf("key error: \"%s\" not found in object", d.key)
}

func JsonPathLookup(obj interface{}, jpath string) (interface{}, error) {
	c, err := Compile(jpath)
	if err != nil {
		return nil, err
	}
	return c.Lookup(obj)
}

type Compiled struct {
	path  string
	steps []step
}

type step struct {
	op   string
	key  string
	args interface{}
}

func MustCompile(jpath string) *Compiled {
	c, err := Compile(jpath)
	if err != nil {
		panic(err)
	}
	return c
}

func Compile(jpath string) (*Compiled, error) {
	tokens, err := tokenize(jpath)
	if err != nil {
		return nil, err
	}
	if tokens[0] != "@" && tokens[0] != "$" {
		return nil, fmt.Errorf("$ or @ should in front of path")
	}
	tokens = tokens[1:]
	res := Compiled{
		path:  jpath,
		steps: make([]step, len(tokens)),
	}
	for i, token := range tokens {
		op, key, args, err := parse_token(token)
		if err != nil {
			return nil, err
		}
		res.steps[i] = step{op, key, args}
	}
	return &res, nil
}

func (c *Compiled) String() string {
	return fmt.Sprintf("Compiled lookup: %s", c.path)
}

// lookupKey find key and return object by key.
// If step contain subkey then child object of founded object will be returned by subkey
func (c *Compiled) lookupKey(obj interface{}, s step) (interface{}, error) {
	subKey, ok := s.args.(string)
	if !ok {
		subKey = ""
	}
	if s.key == "" && subKey != "" {
		s.key = subKey
		subKey = ""
	}
	obj, err := get_key(obj, s.key)
	if err != nil {
		return nil, err
	}
	if subKey != "" {
		obj, err = get_key(obj, subKey)
	}
	return obj, err
}

func (c *Compiled) Lookup(rootObj interface{}) (interface{}, error) {
	obj := rootObj
	var err error
	for _, s := range c.steps {
		// "key", "idx"
		switch s.op {
		case KeyOp:
			obj, err = c.lookupKey(obj, s)
			if err != nil {
				return nil, err
			}
		case IndexOp:
			if len(s.key) > 0 {
				// no key `$[0].test`
				obj, err = get_key(obj, s.key)
				if err != nil {
					return nil, err
				}
			}

			if len(s.args.([]int)) > 1 {
				res := []interface{}{}
				for _, x := range s.args.([]int) {
					//fmt.Println("idx ---- ", x)
					tmp, err := get_idx(obj, x)
					if err != nil {
						return nil, err
					}
					res = append(res, tmp)
				}
				obj = res
			} else if len(s.args.([]int)) == 1 {
				//fmt.Println("idx ----------------3")
				obj, err = get_idx(obj, s.args.([]int)[0])
				if err != nil {
					return nil, err
				}
			} else {
				//fmt.Println("idx ----------------4")
				return nil, fmt.Errorf("cannot index on empty slice")
			}
		case RangeOp:
			if len(s.key) > 0 {
				// no key `$[:1].test`
				obj, err = get_key(obj, s.key)
				if err != nil {
					return nil, err
				}
			}
			if argsv, ok := s.args.([2]interface{}); ok {
				obj, err = get_range(obj, argsv[0], argsv[1])
				if err != nil {
					return nil, err
				}
			} else {
				return nil, fmt.Errorf("range args length should be 2")
			}
		case FilterOp:
			obj, err = get_key(obj, s.key)
			if err != nil {
				return nil, err
			}
			obj, err = get_filtered(obj, rootObj, s.args.(string))
			if err != nil {
				return nil, err
			}
		case "scan":
			return obj, nil
		default:
			return nil, fmt.Errorf("expression don't support in filter")
		}
	}
	return obj, nil
}

func tokenize(query string) ([]string, error) {
	tokens := []string{}
	//	token_start := false
	//	token_end := false
	token := ""

	// fmt.Println("-------------------------------------------------- start")
	for idx, x := range query {
		token += string(x)
		// //fmt.Printf("idx: %d, x: %s, token: %s, tokens: %v\n", idx, string(x), token, tokens)
		if idx == 0 {
			if token == "$" || token == "@" {
				tokens = append(tokens, token[:])
				token = ""
				continue
			} else {
				return nil, fmt.Errorf("should start with '$'")
			}
		}
		if token == "." {
			continue
		} else if token == ".." {
			if tokens[len(tokens)-1] != "*" {
				tokens = append(tokens, "*")
			}
			token = "."
			continue
		} else {
			// fmt.Println("else: ", string(x), token)
			if strings.Contains(token, "[") {
				// fmt.Println(" contains [ ")
				if x == ']' && !strings.HasSuffix(token, "\\]") {
					if token[0] == '.' {
						tokens = append(tokens, token[1:])
					} else {
						tokens = append(tokens, token[:])
					}
					token = ""
					continue
				}
			} else {
				// fmt.Println(" doesn't contains [ ")
				if x == '.' {
					if token[0] == '.' {
						tokens = append(tokens, token[1:len(token)-1])
					} else {
						tokens = append(tokens, token[:len(token)-1])
					}
					token = "."
					continue
				}
			}
		}
	}
	if len(token) > 0 {
		if token[0] == '.' {
			token = token[1:]
			if token != "*" {
				tokens = append(tokens, token[:])
			} else if tokens[len(tokens)-1] != "*" {
				tokens = append(tokens, token[:])
			}
		} else {
			if token != "*" {
				tokens = append(tokens, token[:])
			} else if tokens[len(tokens)-1] != "*" {
				tokens = append(tokens, token[:])
			}
		}
	}
	// fmt.Println("finished tokens: ", tokens)
	// fmt.Println("================================================= done ")
	return tokens, nil
}

func checkFilter(filter string) bool {
	return strings.HasPrefix(filter, "@.") || strings.HasPrefix(filter, "$.")
}

/*
 op: "root", "key", "idx", "range", "filter", "scan"
*/
func parse_token(token string) (op string, key string, args interface{}, err error) {
	if token == "$" {
		return "root", "$", nil, nil
	}
	if token == "*" {
		return "scan", "*", nil, nil
	}

	bracket_idx := strings.Index(token, "[")
	if bracket_idx < 0 {
		return "key", token, nil, nil
	} else {
		key = token[:bracket_idx]
		tail := token[bracket_idx:]
		if len(tail) < 3 {
			err = fmt.Errorf("len(tail) should >=3, %v", tail)
			return
		}
		tail = tail[1 : len(tail)-1]

		// for ['some.key']
		if tail[0] == 39 {
			return "key", key, tail[1 : len(tail)-1], nil
		}
		if strings.Contains(tail, "?") {
			// filter -------------------------------------------------
			op = FilterOp
			first := strings.Index(tail, "(")
			last := strings.LastIndex(tail, ")")
			if first == -1 || last == -1 {
				err = fmt.Errorf("invalid filter must contains parenthesis: %v", tail)
				op = ""
			} else {
				filter := strings.TrimSpace(tail[first+1 : last])
				if !checkFilter(filter) {
					err = fmt.Errorf("invalid filter: %v", tail)
				}
				args = filter
			}
			return
		} else if strings.Contains(tail, ":") {
			// range ----------------------------------------------
			op = RangeOp
			tails := strings.Split(tail, ":")
			if len(tails) != 2 {
				err = fmt.Errorf("only support one range(from, to): %v", tails)
				return
			}
			var frm interface{}
			var to interface{}
			if frm, err = strconv.Atoi(strings.Trim(tails[0], " ")); err != nil {
				if strings.Trim(tails[0], " ") == "" {
					err = nil
				}
				frm = nil
			}
			if to, err = strconv.Atoi(strings.Trim(tails[1], " ")); err != nil {
				if strings.Trim(tails[1], " ") == "" {
					err = nil
				}
				to = nil
			}
			args = [2]interface{}{frm, to}
			return
		} else if tail == "*" {
			op = RangeOp
			args = [2]interface{}{nil, nil}
			return
		} else {
			// idx ------------------------------------------------
			op = IndexOp
			res := []int{}
			for _, x := range strings.Split(tail, ",") {
				if i, err := strconv.Atoi(strings.Trim(x, " ")); err == nil {
					res = append(res, i)
				} else {
					return "", "", nil, err
				}
			}
			args = res
		}
	}
	return op, key, args, nil
}

func filter_get_from_explicit_path(obj interface{}, path string) (interface{}, error) {
	steps, err := tokenize(path)
	//fmt.Println("f: steps: ", steps, err)
	//fmt.Println(path, steps)
	if err != nil {
		return nil, err
	}
	if steps[0] != "@" && steps[0] != "$" {
		return nil, fmt.Errorf("$ or @ should in front of path")
	}
	steps = steps[1:]
	xobj := obj
	//fmt.Println("f: xobj", xobj)
	for _, s := range steps {
		op, key, args, err := parse_token(s)
		// "key", "idx"
		switch op {
		case KeyOp:
			xobj, err = get_key(xobj, key)
			if err != nil {
				return nil, err
			}
		case IndexOp:
			if len(args.([]int)) != 1 {
				return nil, fmt.Errorf("don't support multiple index in filter")
			}
			xobj, err = get_key(xobj, key)
			if err != nil {
				return nil, err
			}
			xobj, err = get_idx(xobj, args.([]int)[0])
			if err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("expression don't support in filter")
		}
	}
	return xobj, nil
}

func get_key(obj interface{}, key string) (interface{}, error) {
	if reflect.TypeOf(obj) == nil {
		return nil, ErrGetFromNullObj
	}
	obj = followPtr(obj)
	objType := reflect.TypeOf(obj)
	switch objType.Kind() {
	case reflect.Map:
		// if obj came from stdlib json, its highly likely to be a map[string]interface{}
		// in which case we can save having to iterate the map keys to work out if the
		// key exists
		if jsonMap, ok := obj.(map[string]interface{}); ok {
			val, exists := jsonMap[key]
			if !exists {
				return nil, NotExist{key: key}
			}
			return val, nil
		}
		for _, kv := range reflect.ValueOf(obj).MapKeys() {
			//fmt.Println(kv.String())
			if kv.String() == key {
				return reflect.ValueOf(obj).MapIndex(kv).Interface(), nil
			}
		}
		return nil, NotExist{key: key}
	case reflect.Slice:
		// slice we should get from all objects in it.
		res := []interface{}{}
		for i := 0; i < reflect.ValueOf(obj).Len(); i++ {
			tmp, _ := get_idx(obj, i)
			if v, err := get_key(tmp, key); err == nil {
				res = append(res, v)
			}
		}
		if len(res) == 0 {
			return nil, NotExist{key: key}
		}
		return res, nil
	default:
		return nil, fmt.Errorf("object is not map or slice")
	}
}

func get_idx(obj interface{}, idx int) (interface{}, error) {
	if reflect.TypeOf(obj).Kind() != reflect.Slice {
		return nil, fmt.Errorf("object is not Slice")
	}
	length := reflect.ValueOf(obj).Len()
	if idx >= 0 {
		if idx >= length {
			return nil, fmt.Errorf("index out of range: len: %v, idx: %v", length, idx)
		}
		return reflect.ValueOf(obj).Index(idx).Interface(), nil
	} else {
		// < 0
		_idx := length + idx
		if _idx < 0 {
			return nil, fmt.Errorf("index out of range: len: %v, idx: %v", length, idx)
		}
		return reflect.ValueOf(obj).Index(_idx).Interface(), nil
	}
}

func get_range(obj, frm, to interface{}) (interface{}, error) {
	switch reflect.TypeOf(obj).Kind() {
	case reflect.Slice:
		length := reflect.ValueOf(obj).Len()
		_frm := 0
		_to := length
		if frm == nil {
			frm = 0
		}
		if to == nil {
			to = length - 1
		}
		if fv, ok := frm.(int); ok {
			if fv < 0 {
				_frm = length + fv
			} else {
				_frm = fv
			}
		}
		if tv, ok := to.(int); ok {
			if tv < 0 {
				_to = length + tv + 1
			} else {
				_to = tv + 1
			}
		}
		if _frm < 0 || _frm >= length {
			return nil, fmt.Errorf("index [from] out of range: len: %v, from: %v", length, frm)
		}
		if _to < 0 || _to > length {
			return nil, fmt.Errorf("index [to] out of range: len: %v, to: %v", length, to)
		}
		//fmt.Println("_frm, _to: ", _frm, _to)
		res_v := reflect.ValueOf(obj).Slice(_frm, _to)
		return res_v.Interface(), nil
	default:
		return nil, fmt.Errorf("object is not Slice")
	}
}

func regFilterCompile(rule string) (*regexp.Regexp, error) {
	runes := []rune(rule)
	if len(runes) <= 2 {
		return nil, errors.New("empty rule")
	}

	if runes[0] != '/' || runes[len(runes)-1] != '/' {
		return nil, errors.New("invalid syntax. should be in `/pattern/` form")
	}
	runes = runes[1 : len(runes)-1]
	return regexp.Compile(string(runes))
}

func get_filtered(obj, root interface{}, filter string) ([]interface{}, error) {
	lp, op, rp, err := parse_filter(filter)
	if err != nil {
		return nil, err
	}

	res := []interface{}{}

	switch reflect.TypeOf(obj).Kind() {
	case reflect.Slice:
		if op == "=~" {
			// regexp
			pat, err := regFilterCompile(rp)
			if err != nil {
				return nil, err
			}

			for i := 0; i < reflect.ValueOf(obj).Len(); i++ {
				tmp := reflect.ValueOf(obj).Index(i).Interface()
				ok, err := eval_reg_filter(tmp, root, lp, pat)
				if err != nil {
					return nil, err
				}
				if ok {
					res = append(res, tmp)
				}
			}
		} else {
			for i := 0; i < reflect.ValueOf(obj).Len(); i++ {
				tmp := reflect.ValueOf(obj).Index(i).Interface()
				ok, err := eval_filter(tmp, root, lp, op, rp)
				if err != nil {
					return nil, err
				}
				if ok {
					res = append(res, tmp)
				}
			}
		}
		return res, nil
	case reflect.Map:
		if op == "=~" {
			// regexp
			pat, err := regFilterCompile(rp)
			if err != nil {
				return nil, err
			}

			for _, kv := range reflect.ValueOf(obj).MapKeys() {
				tmp := reflect.ValueOf(obj).MapIndex(kv).Interface()
				ok, err := eval_reg_filter(tmp, root, lp, pat)
				if err != nil {
					return nil, err
				}
				if ok {
					res = append(res, tmp)
				}
			}
		} else {
			for _, kv := range reflect.ValueOf(obj).MapKeys() {
				tmp := reflect.ValueOf(obj).MapIndex(kv).Interface()
				ok, err := eval_filter(tmp, root, lp, op, rp)
				if err != nil {
					return nil, err
				}
				if ok {
					res = append(res, tmp)
				}
			}
		}
	default:
		return nil, fmt.Errorf("don't support filter on this type: %v", reflect.TypeOf(obj).Kind())
	}

	return res, nil
}

// @.isbn                 => @.isbn, exists, nil
// @.price < 10           => @.price, <, 10
// @.price <= $.expensive => @.price, <=, $.expensive
// @.author =~ /.*REES/i  => @.author, match, /.*REES/i

func parse_filter(filter string) (lp string, op string, rp string, err error) {
	tmp := ""

	stage := 0
	str_embrace := false
	for idx, c := range filter {
		switch c {
		case '\'':
			if !str_embrace {
				str_embrace = true
			} else {
				switch stage {
				case 0:
					lp = tmp
				case 1:
					op = tmp
				case 2:
					rp = tmp
				}
				tmp = ""
			}
		case ' ':
			if str_embrace {
				tmp += string(c)
				continue
			}
			switch stage {
			case 0:
				lp = tmp
			case 1:
				op = tmp
			case 2:
				rp = tmp
			}
			tmp = ""

			stage += 1
			if stage > 2 {
				return "", "", "", fmt.Errorf(fmt.Sprintf("invalid char at %d: `%c`", idx, c))
			}
		default:
			tmp += string(c)
		}
	}
	if tmp != "" {
		switch stage {
		case 0:
			lp = tmp
			op = "exists"
		case 1:
			op = tmp
		case 2:
			rp = tmp
		}
		tmp = ""
	}
	return lp, op, rp, err
}

func parse_filter_v1(filter string) (lp string, op string, rp string, err error) {
	tmp := ""
	istoken := false
	for _, c := range filter {
		if !istoken && c != ' ' {
			istoken = true
		}
		if istoken && c == ' ' {
			istoken = false
		}
		if istoken {
			tmp += string(c)
		}
		if !istoken && tmp != "" {
			if lp == "" {
				lp = tmp[:]
				tmp = ""
			} else if op == "" {
				op = tmp[:]
				tmp = ""
			} else if rp == "" {
				rp = tmp[:]
				tmp = ""
			}
		}
	}
	if tmp != "" && lp == "" && op == "" && rp == "" {
		lp = tmp[:]
		op = "exists"
		rp = ""
		err = nil
		return
	} else if tmp != "" && rp == "" {
		rp = tmp[:]
		tmp = ""
	}
	return lp, op, rp, err
}

func eval_reg_filter(obj, root interface{}, lp string, pat *regexp.Regexp) (res bool, err error) {
	if pat == nil {
		return false, errors.New("nil pat")
	}
	lp_v, err := get_lp_v(obj, root, lp)
	if err != nil {
		return false, err
	}
	switch v := lp_v.(type) {
	case string:
		return pat.MatchString(v), nil
	default:
		return false, errors.New("only string can match with regular expression")
	}
}

func get_lp_v(obj, root interface{}, lp string) (interface{}, error) {
	if strings.HasPrefix(lp, "@.") {
		return filter_get_from_explicit_path(obj, lp)
	} else if strings.HasPrefix(lp, "$.") {
		return filter_get_from_explicit_path(root, lp)
	} else {
		return nil, fmt.Errorf("invalid filter %s", lp)
	}
}

func eval_filter(obj, root interface{}, lp, op, rp string) (res bool, err error) {
	lp_v, err := get_lp_v(obj, root, lp)

	if op == "exists" {
		return lp_v != nil, nil
	} else if op == "=~" {
		return false, fmt.Errorf("not implemented yet")
	} else {
		var rp_v interface{}
		if strings.HasPrefix(rp, "@.") {
			rp_v, err = filter_get_from_explicit_path(obj, rp)
		} else if strings.HasPrefix(rp, "$.") {
			rp_v, err = filter_get_from_explicit_path(root, rp)
		} else {
			rp_v = rp
		}
		//fmt.Printf("lp_v: %v, rp_v: %v\n", lp_v, rp_v)
		return cmp_any(lp_v, rp_v, op)
	}
}

func isNumber(o interface{}) bool {
	switch v := o.(type) {
	case int, int8, int16, int32, int64:
		return true
	case uint, uint8, uint16, uint32, uint64:
		return true
	case float32, float64:
		return true
	case string:
		_, err := strconv.ParseFloat(v, 64)
		if err == nil {
			return true
		} else {
			return false
		}
	}
	return false
}

func cmp_any(obj1, obj2 interface{}, op string) (bool, error) {
	switch op {
	case "<", "<=", "==", ">=", ">":
	default:
		return false, fmt.Errorf("invalid filter operation \"%s\";should be one of: <, <=, ==, >= and >", op)
	}

	var exp string
	if isNumber(obj1) && isNumber(obj2) {
		exp = fmt.Sprintf(`%v %s %v`, obj1, op, obj2)
	} else {
		exp = fmt.Sprintf(`"%v" %s "%v"`, obj1, op, obj2)
	}
	//fmt.Println("exp: ", exp)
	fset := token.NewFileSet()
	res, err := types.Eval(fset, nil, 0, exp)
	if err != nil {
		return false, err
	}
	if !res.IsValue() || (res.Value.String() != "false" && res.Value.String() != "true") {
		return false, fmt.Errorf("result should only be true or false")
	}
	if res.Value.String() == "true" {
		return true, nil
	}

	return false, nil
}

func followPtr(data interface{}) interface{} {
	rv := reflect.ValueOf(data)
	for rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	return rv.Interface()
}

func Set(obj interface{}, path string, value interface{}) error {
	c, err := Compile(path)
	if err != nil {
		return err
	}

	obj = followPtr(obj)
	value = followPtr(value)

	child := obj
	parent := obj

	lastStepIdx := len(c.steps) - 1

	for i, s := range c.steps {
		switch s.op {
		case KeyOp:
			child, err = get_key(parent, s.key)
			if err != nil {
				if _, ok := err.(NotExist); !ok && err != ErrGetFromNullObj {
					return err
				}
				if i != lastStepIdx {
					return fmt.Errorf("incorrect set path %s", path)
				}
			}

		case IndexOp:
			if len(s.key) > 0 {
				// no key `$[0].test`
				parent, err = get_key(parent, s.key)
				if err != nil {
					return err
				}
			}
			if len(s.args.([]int)) == 1 {
				//fmt.Println("idx ----------------3")
				child, err = get_idx(parent, s.args.([]int)[0])
				if err != nil {
					return err
				}
			}

		default:
			return fmt.Errorf("%s expression don't support in set", s.op)
		}

		if i != lastStepIdx {
			parent = child
		}
	}

	last := c.steps[lastStepIdx]
	switch reflect.ValueOf(parent).Kind() {
	case reflect.Map:
		reflect.ValueOf(parent).SetMapIndex(reflect.ValueOf(last.key), reflect.ValueOf(value))
		return nil
	case reflect.Slice:
		sliceValue := reflect.ValueOf(parent)
		idx := last.args.([]int)[0]
		sliceValue.Index(idx).Set(reflect.ValueOf(value))
		return nil
	default:
		return fmt.Errorf("could not set value at path, %s", path)
	}
}

func Del(obj interface{}, path string) error {
	c, err := Compile(path)
	if err != nil {
		return err
	}

	obj = followPtr(obj)
	child := obj
	parent := obj

	lastStepIdx := len(c.steps) - 1

	for i, s := range c.steps {
		switch s.op {
		case KeyOp:
			child, err = get_key(parent, s.key)
			if err != nil {
				return err
			}
		case IndexOp:
			if len(s.key) > 0 {
				// no key `$[0].test`
				parent, err = get_key(parent, s.key)
				if err != nil {
					return err
				}
			}
			if len(s.args.([]int)) == 1 {
				//fmt.Println("idx ----------------3")
				child, err = get_idx(parent, s.args.([]int)[0])
				if err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("not support del operation %s", s.op)
		}

		if i != lastStepIdx {
			parent = child
		}
	}

	last := c.steps[lastStepIdx]
	switch reflect.ValueOf(parent).Kind() {
	case reflect.Map:
		reflect.ValueOf(parent).SetMapIndex(reflect.ValueOf(last.key), reflect.Value{})
	case reflect.Slice:
		sliceValue := reflect.ValueOf(parent)
		idx := last.args.([]int)[0]
		sliceValue.Index(idx).Set(reflect.Value{})
	}
	return nil
}

func Append(obj interface{}, path string, value interface{}) error {
	c, err := Compile(path)
	if err != nil {
		return err
	}

	obj = followPtr(obj)
	child := obj
	parent := obj

	lastStepIdx := len(c.steps) - 1

	for i, s := range c.steps {
		switch s.op {
		case KeyOp:
			child, err = get_key(parent, s.key)
			if err != nil {
				return err
			}
		case IndexOp:
			if len(s.key) > 0 {
				parent, err = get_key(parent, s.key)
				if err != nil {
					return err
				}
			}
			if len(s.args.([]int)) == 1 {
				child, err = get_idx(parent, s.args.([]int)[0])
				if err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("not support append operation %s", s.op)
		}

		if i != lastStepIdx {
			parent = child
		}
	}

	last := c.steps[lastStepIdx]
	childVal := reflect.ValueOf(child)
	if childVal.Kind() != reflect.Slice {
		return fmt.Errorf("not support append operation for %v", child)

	}
	newSlice := reflect.Append(childVal, reflect.ValueOf(value))

	switch reflect.ValueOf(parent).Kind() {
	case reflect.Map:
		reflect.ValueOf(parent).SetMapIndex(reflect.ValueOf(last.key), newSlice)
	case reflect.Slice:
		sliceValue := reflect.ValueOf(parent)
		idx := last.args.([]int)[0]
		sliceValue.Index(idx).Set(newSlice)
	}

	return nil

}
