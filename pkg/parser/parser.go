package parser

import "fmt"

type Statement interface{}

type SelectStatement struct {
	Fields []string
	Table  string
}

// Parse parses a SQL string into a Statement AST.
// This is a stub.
func Parse(sql string) (Statement, error) {
	return nil, fmt.Errorf("parser not implemented")
}
