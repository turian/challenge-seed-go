package parser

import (
	"errors"
	"strings"
)

type Statement interface{}

type SelectStatement struct {
	Fields []string
	From   string
	Where  string
}

type Literal string

var (
	ErrParseFail = errors.New("parser failed")
)

// Parse parses a SQL string into a Statement AST.
func Parse(sql string) (Statement, error) {
	sql = strings.TrimSpace(sql)
	if strings.HasPrefix(strings.ToUpper(sql), "SELECT ") {
		fields, from, where, err := parseSelect(sql)
		if err != nil {
			return nil, err
		}
		return SelectStatement{
			Fields: fields,
			From:   from,
			Where:  where,
		}, nil
	}

	return nil, ErrParseFail
}

func parseSelect(sql string) ([]string, string, string, error) {
	sql = strings.TrimPrefix(strings.ToUpper(sql), "SELECT ")
	sql = strings.TrimSpace(sql)
	fieldsEnd := strings.Index(sql, " FROM ")
	if fieldsEnd == -1 {
		return nil, "", "", ErrParseFail
	}

	fieldsStr := sql[:fieldsEnd]
	fields := strings.Split(fieldsStr, ",")
	for i := range fields {
		fields[i] = strings.TrimSpace(fields[i])
	}

	fromClauseStart := fieldsEnd + len(" FROM ")
	whereClauseStart := strings.Index(sql[fromClauseStart:], " WHERE ")

	var fromClause, whereClause string

	if whereClauseStart != -1 {
		whereClauseStart += fromClauseStart
		fromClause = strings.TrimSpace(sql[fromClauseStart:whereClauseStart])
		whereClause = strings.TrimSpace(sql[whereClauseStart+len(" WHERE "):])
	} else {
		fromClause = strings.TrimSpace(sql[fromClauseStart:])
		whereClause = ""
	}

	return fields, fromClause, whereClause, nil
}
