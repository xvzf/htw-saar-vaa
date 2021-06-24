package node

import (
	"errors"
	"strconv"
	"strings"
)

// nthInt extracts an integer out of a string separated by `;`
func nthInt(s string, i int) (int, error) {
	ss := strings.Split(s, ";")
	if len(ss) < i {
		return 0, errors.New("array too short")
	}
	if ri, err := strconv.Atoi(ss[i]); err != nil {
		return 0, err
	} else {
		return ri, nil
	}
}

// nthString extracts an string out of a string separated by `;`
func nthString(s string, i int) (string, error) {
	ss := strings.Split(s, ";")
	if len(ss) <= i {
		return "", errors.New("array too short")
	}
	return ss[i], nil
}

// nthBool extracts an string out of a string separated by `;`
func nthBool(s string, i int) (bool, error) {
	ss := strings.Split(s, ";")
	if len(ss) < i {
		return false, errors.New("array too short")
	}
	return ss[i] == "true", nil
}
