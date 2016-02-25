package main

import "fmt"

type StringSliceFlag struct {
	Values []string
}

func (flag *StringSliceFlag) String() string {
	return fmt.Sprint(flag.Values)
}

func (flag *StringSliceFlag) Set(value string) error {
	flag.Values = append(flag.Values, value)
	return nil
}
