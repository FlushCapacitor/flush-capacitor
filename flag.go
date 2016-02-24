package main

type StringSliceFlag struct {
	Values []string
}

func (flag *StringSliceFlag) String() string {
	return fmt.Sprint(flag.values)
}

func (flag *StringSliceFlag) Set(value string) error {
	flag.values = append(flag.values, value)
}
