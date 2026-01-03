package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	sltSortNoSort    = "nosort"
	sltSortRowSort   = "rowsort"
	sltSortValueSort = "valuesort"
)

type sltRecordKind string

const (
	sltRecordStatement sltRecordKind = "statement"
	sltRecordQuery     sltRecordKind = "query"
	sltRecordControl   sltRecordKind = "control"
)

type sltStatement struct {
	Expected  string
	ErrorHint string
	SQL       string
}

type sltQuery struct {
	TypeString string
	SortMode   string
	Label      string
	SQL        string
	Expected   []string
}

type sltControl struct {
	Kind string
	Arg  string
}

type sltRecord struct {
	Index     int
	Kind      sltRecordKind
	Statement *sltStatement
	Query     *sltQuery
	Control   *sltControl
}

type sltFail struct {
	SchemaVersion        int                      `json:"schema_version"`
	TestFile             string                   `json:"test_file"`
	RecordIndex          int                      `json:"record_index"`
	RecordKind           string                   `json:"record_kind"`
	Label                string                   `json:"label,omitempty"`
	SQL                  string                   `json:"sql"`
	Phase                string                   `json:"phase"`
	StatementExpectation *sltStatementExpectation `json:"statement_expectation,omitempty"`
	QueryMeta            *sltQueryMeta            `json:"query_meta,omitempty"`
	Expected             *sltOutcome              `json:"expected,omitempty"`
	Actual               *sltOutcome              `json:"actual,omitempty"`
	Comparison           *sltComparison           `json:"comparison,omitempty"`
	Failure              sltFailure               `json:"failure"`
	Timing               sltTiming                `json:"timing"`
	Debug                *sltDebug                `json:"debug,omitempty"`
}

type sltStatementExpectation struct {
	ExpectedOutcome   string `json:"expected_outcome"`
	ExpectedErrorHint string `json:"expected_error_hint,omitempty"`
}

type sltQueryMeta struct {
	TypeString string `json:"type_string"`
	SortMode   string `json:"sort_mode"`
	NCols      int    `json:"ncols"`
}

type sltOutcome struct {
	Outcome    string   `json:"outcome"`
	ValueCount int      `json:"value_count,omitempty"`
	Head       []string `json:"head,omitempty"`
	Tail       []string `json:"tail,omitempty"`
	HashSHA256 string   `json:"hash_sha256,omitempty"`
	Error      string   `json:"error,omitempty"`
}

type sltComparison struct {
	FirstDiffIndex int      `json:"first_diff_index"`
	ExpectedAtDiff string   `json:"expected_at_diff,omitempty"`
	ActualAtDiff   string   `json:"actual_at_diff,omitempty"`
	ExpectedWindow []string `json:"expected_window,omitempty"`
	ActualWindow   []string `json:"actual_window,omitempty"`
}

type sltFailure struct {
	Kind    string      `json:"kind"`
	Message string      `json:"message"`
	Details interface{} `json:"details,omitempty"`
}

type sltTiming struct {
	ElapsedMS int64 `json:"elapsed_ms"`
	TimeoutMS int64 `json:"timeout_ms"`
}

type sltDebug struct {
	PlanSummary   string            `json:"plan_summary,omitempty"`
	OperatorStats []sltOperatorStat `json:"operator_stats,omitempty"`
}

type sltOperatorStat struct {
	Op      string `json:"op"`
	InRows  int    `json:"in_rows,omitempty"`
	OutRows int    `json:"out_rows"`
}

type sltParseError struct {
	Index int
	Err   error
}

func (e sltParseError) Error() string {
	return fmt.Sprintf("record %d: %v", e.Index, e.Err)
}

func runSQLLogicTestFile(path string) int {
	records, err := parseSQLLogicTestFile(path)
	if err != nil {
		recordIndex := 0
		if perr, ok := err.(sltParseError); ok {
			recordIndex = perr.Index
		}
		emitSLTFail(sltFail{
			SchemaVersion: 1,
			TestFile:      relPath(path),
			RecordIndex:   recordIndex,
			RecordKind:    "statement",
			SQL:           "",
			Phase:         "harness",
			Failure: sltFailure{
				Kind:    "harness_error",
				Message: err.Error(),
			},
			Timing: sltTiming{ElapsedMS: 0, TimeoutMS: 0},
		})
		return 1
	}

	for _, rec := range records {
		switch rec.Kind {
		case sltRecordControl:
			if rec.Control != nil && rec.Control.Kind == "halt" {
				return 0
			}
			continue
		case sltRecordStatement:
			if rec.Statement == nil {
				continue
			}
			start := time.Now()
			err := execStatement(rec.Statement.SQL)
			elapsed := time.Since(start).Milliseconds()
			if err != nil {
				if rec.Statement.Expected == "error" {
					continue
				}
				fail := makeStatementFail(path, rec, err.Error(), elapsed, "expected_ok_got_error")
				emitSLTFail(fail)
				return 1
			}
			if rec.Statement.Expected == "error" {
				fail := makeStatementFail(path, rec, "", elapsed, "expected_error_got_ok")
				emitSLTFail(fail)
				return 1
			}
		case sltRecordQuery:
			if rec.Query == nil {
				continue
			}
			start := time.Now()
			rows, err := execQuery(rec.Query.SQL)
			elapsed := time.Since(start).Milliseconds()
			if err != nil {
				fail := makeQueryFail(path, rec, nil, err.Error(), elapsed, "expected_ok_got_error", "exec")
				emitSLTFail(fail)
				return 1
			}
			ok, expectedFlat, actualFlat, compareErr := compareQueryResults(rec.Query, rows)
			if compareErr != nil {
				fail := makeQueryFail(path, rec, nil, compareErr.Error(), elapsed, "harness_error", "harness")
				emitSLTFail(fail)
				return 1
			}
			if !ok {
				fail := makeQueryMismatchFail(path, rec, expectedFlat, actualFlat, elapsed)
				emitSLTFail(fail)
				return 1
			}
		}
	}

	return 0
}

func parseSQLLogicTestFile(path string) ([]sltRecord, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var records [][]string
	var cur []string

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			if len(cur) > 0 {
				records = append(records, cur)
				cur = nil
			}
			continue
		}
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		cur = append(cur, line)
	}
	if len(cur) > 0 {
		records = append(records, cur)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	parsed := make([]sltRecord, 0, len(records))
	for i, lines := range records {
		rec, err := parseSQLLogicTestRecord(i, lines)
		if err != nil {
			return nil, sltParseError{Index: i, Err: err}
		}
		parsed = append(parsed, rec)
	}
	return parsed, nil
}

func parseSQLLogicTestRecord(index int, lines []string) (sltRecord, error) {
	if len(lines) == 0 {
		return sltRecord{Index: index, Kind: sltRecordControl}, nil
	}
	header := strings.Fields(lines[0])
	if len(header) == 0 {
		return sltRecord{Index: index, Kind: sltRecordControl}, nil
	}
	switch header[0] {
	case "statement":
		if len(header) < 2 {
			return sltRecord{}, fmt.Errorf("statement missing expected outcome")
		}
		expected := header[1]
		hint := ""
		if len(header) > 2 {
			hint = strings.Join(header[2:], " ")
		}
		sql := strings.TrimSpace(strings.Join(lines[1:], "\n"))
		return sltRecord{
			Index: index,
			Kind:  sltRecordStatement,
			Statement: &sltStatement{
				Expected:  expected,
				ErrorHint: hint,
				SQL:       sql,
			},
		}, nil
	case "query":
		if len(header) < 2 {
			return sltRecord{}, fmt.Errorf("query missing type string")
		}
		typeString := header[1]
		sortMode := sltSortNoSort
		label := ""
		if len(header) >= 3 {
			if isSortMode(header[2]) {
				sortMode = header[2]
				if len(header) > 3 {
					label = header[3]
				}
			} else {
				label = header[2]
			}
		}
		sqlLines, expectedLines := splitQueryLines(lines[1:])
		sql := strings.TrimSpace(strings.Join(sqlLines, "\n"))
		return sltRecord{
			Index: index,
			Kind:  sltRecordQuery,
			Query: &sltQuery{
				TypeString: typeString,
				SortMode:   sortMode,
				Label:      label,
				SQL:        sql,
				Expected:   expectedLines,
			},
		}, nil
	case "halt":
		return sltRecord{
			Index:   index,
			Kind:    sltRecordControl,
			Control: &sltControl{Kind: "halt"},
		}, nil
	case "skipif", "onlyif", "hash-threshold":
		arg := ""
		if len(header) > 1 {
			arg = header[1]
		}
		return sltRecord{
			Index:   index,
			Kind:    sltRecordControl,
			Control: &sltControl{Kind: header[0], Arg: arg},
		}, nil
	default:
		return sltRecord{}, fmt.Errorf("unknown record type %q", header[0])
	}
}

func splitQueryLines(lines []string) ([]string, []string) {
	sep := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == "----" {
			sep = i
			break
		}
	}
	if sep == -1 {
		return lines, nil
	}
	return lines[:sep], lines[sep+1:]
}

func isSortMode(value string) bool {
	return value == sltSortNoSort || value == sltSortRowSort || value == sltSortValueSort
}

func compareQueryResults(q *sltQuery, actualRows [][]string) (bool, []string, []string, error) {
	ncols := len(q.TypeString)
	expectedRows, err := chunkValues(q.Expected, ncols)
	if err != nil {
		return false, nil, nil, err
	}

	switch q.SortMode {
	case sltSortRowSort:
		sortRows(expectedRows)
		sortRows(actualRows)
	case sltSortValueSort:
		expectedFlat := flattenRows(expectedRows)
		actualFlat := flattenRows(actualRows)
		sort.Strings(expectedFlat)
		sort.Strings(actualFlat)
		return equalSlices(expectedFlat, actualFlat), expectedFlat, actualFlat, nil
	}

	expectedFlat := flattenRows(expectedRows)
	actualFlat := flattenRows(actualRows)
	return equalSlices(expectedFlat, actualFlat), expectedFlat, actualFlat, nil
}

func chunkValues(values []string, ncols int) ([][]string, error) {
	if ncols == 0 {
		if len(values) == 0 {
			return [][]string{}, nil
		}
		return nil, fmt.Errorf("expected values with zero columns")
	}
	if len(values)%ncols != 0 {
		return nil, fmt.Errorf("expected values count %d not divisible by columns %d", len(values), ncols)
	}
	rows := make([][]string, 0, len(values)/ncols)
	for i := 0; i < len(values); i += ncols {
		row := append([]string(nil), values[i:i+ncols]...)
		rows = append(rows, row)
	}
	return rows, nil
}

func sortRows(rows [][]string) {
	sort.Slice(rows, func(i, j int) bool {
		a := rows[i]
		b := rows[j]
		for k := 0; k < len(a) && k < len(b); k++ {
			if a[k] == b[k] {
				continue
			}
			return a[k] < b[k]
		}
		return len(a) < len(b)
	})
}

func flattenRows(rows [][]string) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row...)
	}
	return out
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func makeStatementFail(path string, rec sltRecord, errMsg string, elapsed int64, kind string) sltFail {
	expectation := sltStatementExpectation{
		ExpectedOutcome:   rec.Statement.Expected,
		ExpectedErrorHint: rec.Statement.ErrorHint,
	}
	expected := &sltOutcome{Outcome: rec.Statement.Expected}
	actual := &sltOutcome{}
	if errMsg != "" {
		actual.Outcome = "error"
		actual.Error = errMsg
	} else {
		actual.Outcome = "ok"
	}
	return sltFail{
		SchemaVersion:        1,
		TestFile:             relPath(path),
		RecordIndex:          rec.Index,
		RecordKind:           string(rec.Kind),
		SQL:                  rec.Statement.SQL,
		Phase:                "exec",
		StatementExpectation: &expectation,
		Expected:             expected,
		Actual:               actual,
		Failure: sltFailure{
			Kind:    kind,
			Message: statementFailureMessage(kind),
		},
		Timing: sltTiming{ElapsedMS: elapsed, TimeoutMS: 0},
	}
}

func makeQueryFail(path string, rec sltRecord, actualValues [][]string, errMsg string, elapsed int64, kind string, phase string) sltFail {
	meta := sltQueryMeta{
		TypeString: rec.Query.TypeString,
		SortMode:   rec.Query.SortMode,
		NCols:      len(rec.Query.TypeString),
	}
	expected := summarizeExpected(rec.Query.Expected)
	actual := &sltOutcome{Outcome: "error", Error: errMsg}
	if actualValues != nil {
		actual = summarizeActual(actualValues)
	}
	return sltFail{
		SchemaVersion: 1,
		TestFile:      relPath(path),
		RecordIndex:   rec.Index,
		RecordKind:    string(rec.Kind),
		Label:         rec.Query.Label,
		SQL:           rec.Query.SQL,
		Phase:         phase,
		QueryMeta:     &meta,
		Expected:      expected,
		Actual:        actual,
		Failure: sltFailure{
			Kind:    kind,
			Message: queryFailureMessage(kind),
		},
		Timing: sltTiming{ElapsedMS: elapsed, TimeoutMS: 0},
	}
}

func makeQueryMismatchFail(path string, rec sltRecord, expected []string, actual []string, elapsed int64) sltFail {
	meta := sltQueryMeta{
		TypeString: rec.Query.TypeString,
		SortMode:   rec.Query.SortMode,
		NCols:      len(rec.Query.TypeString),
	}
	comp := firstDiff(expected, actual)
	return sltFail{
		SchemaVersion: 1,
		TestFile:      relPath(path),
		RecordIndex:   rec.Index,
		RecordKind:    string(rec.Kind),
		Label:         rec.Query.Label,
		SQL:           rec.Query.SQL,
		Phase:         "compare",
		QueryMeta:     &meta,
		Expected:      summarizeFlat(expected),
		Actual:        summarizeFlat(actual),
		Comparison:    comp,
		Failure: sltFailure{
			Kind:    "result_mismatch",
			Message: "first mismatch in rendered values",
		},
		Timing: sltTiming{ElapsedMS: elapsed, TimeoutMS: 0},
	}
}

func summarizeExpected(values []string) *sltOutcome {
	return summarizeFlat(values)
}

func summarizeActual(rows [][]string) *sltOutcome {
	return summarizeFlat(flattenRows(rows))
}

func summarizeFlat(values []string) *sltOutcome {
	out := &sltOutcome{
		Outcome:    "values",
		ValueCount: len(values),
	}
	if len(values) == 0 {
		return out
	}
	head, tail := headTail(values, 5, 5)
	out.Head = head
	out.Tail = tail
	sum := sha256.Sum256([]byte(strings.Join(values, "\n")))
	out.HashSHA256 = fmt.Sprintf("%x", sum[:])
	return out
}

func headTail(values []string, headCount int, tailCount int) ([]string, []string) {
	if len(values) <= headCount+tailCount {
		return append([]string(nil), values...), nil
	}
	head := append([]string(nil), values[:headCount]...)
	tail := append([]string(nil), values[len(values)-tailCount:]...)
	return head, tail
}

func firstDiff(expected []string, actual []string) *sltComparison {
	max := len(expected)
	if len(actual) < max {
		max = len(actual)
	}
	diffIndex := max
	for i := 0; i < max; i++ {
		if expected[i] != actual[i] {
			diffIndex = i
			break
		}
	}
	comp := &sltComparison{
		FirstDiffIndex: diffIndex,
	}
	if diffIndex < len(expected) {
		comp.ExpectedAtDiff = expected[diffIndex]
	}
	if diffIndex < len(actual) {
		comp.ActualAtDiff = actual[diffIndex]
	}
	if diffIndex != max {
		comp.ExpectedWindow = window(expected, diffIndex, 2)
		comp.ActualWindow = window(actual, diffIndex, 2)
	}
	return comp
}

func window(values []string, idx int, radius int) []string {
	start := idx - radius
	if start < 0 {
		start = 0
	}
	end := idx + radius + 1
	if end > len(values) {
		end = len(values)
	}
	return append([]string(nil), values[start:end]...)
}

func statementFailureMessage(kind string) string {
	switch kind {
	case "expected_error_got_ok":
		return "expected error but statement succeeded"
	case "expected_ok_got_error":
		return "expected ok but statement returned error"
	default:
		return "statement failure"
	}
}

func queryFailureMessage(kind string) string {
	switch kind {
	case "expected_ok_got_error":
		return "expected values but query returned error"
	default:
		return "query failure"
	}
}

func emitSLTFail(fail sltFail) {
	data, err := json.Marshal(fail)
	if err != nil {
		fmt.Fprintf(os.Stderr, "SLT_FAIL: {\"schema_version\":1,\"failure\":{\"kind\":\"harness_error\",\"message\":\"%s\"}}\n", escapeForJSON(err.Error()))
		return
	}
	fmt.Fprintf(os.Stderr, "SLT_FAIL: %s\n", data)
}

func escapeForJSON(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return replacer.Replace(value)
}

func relPath(path string) string {
	if path == "" {
		return ""
	}
	if rel, err := filepath.Rel(".", path); err == nil {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(path)
}
