**Validation Strategy:**
*   **Unit Level:** Run component-specific tests after each fix (`go test ./pkg/{component}/...`)
*   **Integration Level:** Run integration tests after major component fixes (`go test ./test/integration/...`)
*   **Linting:** Run linter on modified files after each change (`golangci-lint run {files}`)
*   **Focused Testing:** Use pattern matching to test specific functions (`go test ./pkg/... -run TestFunctionName`)
*   **Debug Mode:** 
    *   Use `LOG_LEVEL=DEBUG go test -v ./path/to/package -run TestName` to enable high-level debug flow messages (`[LOG_DEBUG] prefix`).
    *   Use `IRR_DEBUG=1 go test -v ./path/to/package -run TestName` to enable detailed function tracing and value dumps (`[DEBUG +...] prefix`).
    *   Set both variables to enable all debug output.
*   **End-to-End:** Use Python test script on key chart examples after all fixes (`python test/tools/test-charts.py`)

**New Section 26:**
*   **New Section 26:**
    *   **New Section 26:**
        *   **New Section 26:** 