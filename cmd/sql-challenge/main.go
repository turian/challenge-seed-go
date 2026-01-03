package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SQL Vibe Coding Challenge - Go Seed
//
// Your task: Build a SQL database that passes 100% of SQLLogicTest.
//
// This skeleton provides the basic REPL structure. You'll need to:
// 1. Implement a SQL parser
// 2. Build a query executor
// 3. Create storage for tables and indexes
// 4. Handle all SQL operations (SELECT, INSERT, UPDATE, DELETE, etc.)

func main() {
	if len(os.Args) > 1 {
		// File mode: execute SQL from file
		filename := os.Args[1]
		if isSQLLogicTestFile(filename) {
			os.Exit(runSQLLogicTestFile(filename))
		}
		file, err := os.Open(filename)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening file: %v\n", err)
			os.Exit(1)
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		var statement strings.Builder

		for scanner.Scan() {
			line := scanner.Text()
			statement.WriteString(line)
			statement.WriteString("\n")

			// Execute when we hit a semicolon
			if strings.HasSuffix(strings.TrimSpace(line), ";") {
				result, err := execute(statement.String())
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error: %v\n", err)
					os.Exit(1)
				}
				if result != "" {
					fmt.Println(result)
				}
				statement.Reset()
			}
		}

		// Execute any remaining statement
		if statement.Len() > 0 {
			result, err := execute(statement.String())
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			if result != "" {
				fmt.Println(result)
			}
		}
	} else {
		// Interactive REPL mode
		fmt.Println("SQL Challenge REPL (Go)")
		fmt.Println("Type 'exit' or 'quit' to exit.")
		fmt.Println()

		scanner := bufio.NewScanner(os.Stdin)
		var statement strings.Builder

		for {
			if statement.Len() == 0 {
				fmt.Print("sql> ")
			} else {
				fmt.Print("...> ")
			}

			if !scanner.Scan() {
				break
			}

			line := scanner.Text()

			// Check for exit commands
			trimmed := strings.TrimSpace(strings.ToLower(line))
			if statement.Len() == 0 && (trimmed == "exit" || trimmed == "quit") {
				break
			}

			statement.WriteString(line)
			statement.WriteString("\n")

			// Execute when we hit a semicolon
			if strings.HasSuffix(strings.TrimSpace(line), ";") {
				result, err := execute(statement.String())
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				} else if result != "" {
					fmt.Println(result)
				}
				statement.Reset()
			}
		}
	}
}

// execute parses and executes a SQL statement, returning the result as a string.
// This is where you'll implement your SQL database!
func execute(sql string) (string, error) {
	sql = strings.TrimSpace(sql)
	if sql == "" {
		return "", nil
	}

	// TODO: Implement your SQL parser and executor here!
	//
	// For now, this just returns an error for any SQL.
	// Your implementation should:
	// 1. Parse the SQL into an AST
	// 2. Execute the query against your storage engine
	// 3. Return results formatted as tab-separated values
	//
	// Example expected output for "SELECT 1, 2, 3":
	// "1\t2\t3"

	return "", fmt.Errorf("SQL execution not yet implemented")
}

func execStatement(sql string) error {
	_, err := execute(sql)
	return err
}

func execQuery(sql string) ([][]string, error) {
	return nil, fmt.Errorf("SQL query execution not yet implemented")
}

func isSQLLogicTestFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".test"
}
