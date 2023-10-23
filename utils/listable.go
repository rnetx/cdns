package utils

import "encoding/json"

type Listable[T any] []T

func (l *Listable[T]) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var list []T
	err := unmarshal(&list)
	if err == nil {
		*l = list
		return nil
	}
	var single T
	err2 := unmarshal(&single)
	if err2 == nil {
		*l = []T{single}
		return nil
	}
	return err
}

func (l *Listable[T]) MarshalYAML() (interface{}, error) {
	if len(*l) == 1 {
		return (*l)[0], nil
	} else {
		return ([]T)(*l), nil
	}
}

func (l *Listable[T]) UnmarshalJSON(data []byte) error {
	var list []T
	err := json.Unmarshal(data, &list)
	if err == nil {
		*l = list
		return nil
	}
	var single T
	err2 := json.Unmarshal(data, &single)
	if err2 == nil {
		*l = []T{single}
		return nil
	}
	return err
}

func (l *Listable[T]) MarshalJSON() ([]byte, error) {
	if len(*l) == 1 {
		return json.Marshal((*l)[0])
	} else {
		return json.Marshal([]T(*l))
	}
}
