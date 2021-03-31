package iotfwdrv

import (
	"fmt"
	"regexp"
	"strconv"
)

type Version struct {
	Major int `json:"major"`
	Minor int `json:"minor"`
}

func (v Version) GreaterThan(ver Version) bool {
	if v.Major > ver.Major {
		return true
	}
	if v.Major == ver.Major && v.Minor > ver.Minor {
		return true
	}
	return false
}

func (v Version) String() string {
	return fmt.Sprintf("%d.%d", v.Major, v.Minor)
}

func ParseVersion(v string) (Version, error) {
	matches := regexp.MustCompile("([0-9]+)\\.([0-9]+)").FindStringSubmatch(v)
	var ver Version
	var err error
	if len(matches) != 3 {
		err = fmt.Errorf("unexpected match count, got %d", len(matches))
		return ver, err
	}
	ver.Major, err = strconv.Atoi(matches[1])
	if err != nil {
		return ver, err
	}
	ver.Minor, err = strconv.Atoi(matches[2])
	if err != nil {
		return ver, err
	}
	return ver, nil
}
