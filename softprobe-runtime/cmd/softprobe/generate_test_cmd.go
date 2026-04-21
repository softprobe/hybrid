package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// runGenerateTest implements `softprobe generate test --framework {jest,
// vitest,pytest,junit} --case FILE --out FILE`. It emits a **full test
// file** (including describe/it scaffolding) for the target framework,
// rather than just the session helper produced by `generate jest-session`.
// Keep the templates deliberately small and deterministic so regenerating
// the file after a capture refresh produces a clean diff — this is the
// whole reason codegen exists in this workflow.
func runGenerateTest(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("generate test", flag.ContinueOnError)
	fs.SetOutput(stderr)
	casePath := fs.String("case", "", "case file path")
	outPath := fs.String("out", "", "output test file path")
	framework := fs.String("framework", "jest", "target framework: jest|vitest|pytest|junit")
	jsonOut := fs.Bool("json", false, "emit JSON envelope to stdout")
	if err := fs.Parse(args); err != nil {
		return exitInvalidArgs
	}
	if *casePath == "" {
		if *jsonOut {
			writeJSONError(stdout, exitInvalidArgs, "missing_flag", "generate test requires --case", nil)
		} else {
			_, _ = fmt.Fprintln(stderr, "generate test requires --case")
		}
		return exitInvalidArgs
	}
	if *outPath == "" {
		if *jsonOut {
			writeJSONError(stdout, exitInvalidArgs, "missing_flag", "generate test requires --out", nil)
		} else {
			_, _ = fmt.Fprintln(stderr, "generate test requires --out")
		}
		return exitInvalidArgs
	}

	caseBytes, err := os.ReadFile(*casePath)
	if err != nil {
		if *jsonOut {
			writeJSONError(stdout, exitGeneric, "read_case", err.Error(), nil)
		} else {
			_, _ = fmt.Fprintf(stderr, "generate test failed: %v\n", err)
		}
		return exitGeneric
	}

	var doc caseDocument
	if err := json.Unmarshal(caseBytes, &doc); err != nil {
		if *jsonOut {
			writeJSONError(stdout, exitValidation, "invalid_case", err.Error(), nil)
		} else {
			_, _ = fmt.Fprintf(stderr, "generate test failed: invalid case: %v\n", err)
		}
		return exitValidation
	}
	rules := extractJestSessionRules(doc)

	var source string
	var genErr error
	switch strings.ToLower(*framework) {
	case "jest":
		source, genErr = emitJestTestFile(*casePath, *outPath, doc, rules)
	case "vitest":
		source, genErr = emitVitestTestFile(*casePath, *outPath, doc, rules)
	case "pytest":
		source, genErr = emitPytestTestFile(*casePath, *outPath, doc, rules)
	case "junit":
		source, genErr = emitJUnitTestFile(*casePath, *outPath, doc, rules)
	default:
		if *jsonOut {
			writeJSONError(stdout, exitInvalidArgs, "unknown_framework",
				fmt.Sprintf("unknown --framework %q; supported: jest|vitest|pytest|junit", *framework),
				map[string]any{"framework": *framework})
		} else {
			_, _ = fmt.Fprintf(stderr, "unknown --framework %q; supported: jest|vitest|pytest|junit\n", *framework)
		}
		return exitInvalidArgs
	}
	if genErr != nil {
		if *jsonOut {
			writeJSONError(stdout, exitGeneric, "generate_failed", genErr.Error(), nil)
		} else {
			_, _ = fmt.Fprintf(stderr, "generate test failed: %v\n", genErr)
		}
		return exitGeneric
	}

	if err := os.MkdirAll(filepath.Dir(*outPath), 0o755); err != nil {
		if *jsonOut {
			writeJSONError(stdout, exitGeneric, "mkdir_failed", err.Error(), nil)
		} else {
			_, _ = fmt.Fprintf(stderr, "generate test failed: %v\n", err)
		}
		return exitGeneric
	}
	if err := os.WriteFile(*outPath, []byte(source), 0o600); err != nil {
		if *jsonOut {
			writeJSONError(stdout, exitGeneric, "write_failed", err.Error(), nil)
		} else {
			_, _ = fmt.Fprintf(stderr, "generate test failed: %v\n", err)
		}
		return exitGeneric
	}

	if *jsonOut {
		writeJSONEnvelope(stdout, "ok", exitOK, map[string]any{
			"outputPath":   *outPath,
			"framework":    strings.ToLower(*framework),
			"rulesEmitted": len(rules),
			"caseId":       doc.CaseID,
		})
		return exitOK
	}
	_, _ = fmt.Fprintf(stdout, "wrote %s (framework=%s, rules=%d)\n", *outPath, strings.ToLower(*framework), len(rules))
	return exitOK
}

func emitJestTestFile(casePath, outPath string, doc caseDocument, rules []jestSessionRule) (string, error) {
	rel, err := relFromOut(casePath, outPath)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	writeGenHeader(&buf, "// ")
	buf.WriteString("import path from 'path';\n")
	buf.WriteString("import { Softprobe } from '@softprobe/softprobe-js';\n\n")
	fmt.Fprintf(&buf, "describe(%q, () => {\n", describeNameFor(doc))
	buf.WriteString("  let close: (() => Promise<void>) | undefined;\n")
	buf.WriteString("  afterEach(async () => { if (close) await close(); close = undefined; });\n\n")
	fmt.Fprintf(&buf, "  it(%q, async () => {\n", itNameFor(doc))
	buf.WriteString("    const softprobe = new Softprobe();\n")
	buf.WriteString("    const session = await softprobe.startSession({ mode: 'replay' });\n")
	buf.WriteString("    close = () => session.close();\n")
	fmt.Fprintf(&buf, "    await session.loadCaseFromFile(path.join(__dirname, %q));\n", filepath.ToSlash(rel))
	emitTSRules(&buf, rules, "    ")
	buf.WriteString("    // TODO: exercise the system under test and assert.\n")
	buf.WriteString("  });\n")
	buf.WriteString("});\n")
	return buf.String(), nil
}

func emitVitestTestFile(casePath, outPath string, doc caseDocument, rules []jestSessionRule) (string, error) {
	rel, err := relFromOut(casePath, outPath)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	writeGenHeader(&buf, "// ")
	buf.WriteString("import path from 'path';\n")
	buf.WriteString("import { describe, it, afterEach } from 'vitest';\n")
	buf.WriteString("import { Softprobe } from '@softprobe/softprobe-js';\n\n")
	fmt.Fprintf(&buf, "describe(%q, () => {\n", describeNameFor(doc))
	buf.WriteString("  let close: (() => Promise<void>) | undefined;\n")
	buf.WriteString("  afterEach(async () => { if (close) await close(); close = undefined; });\n\n")
	fmt.Fprintf(&buf, "  it(%q, async () => {\n", itNameFor(doc))
	buf.WriteString("    const softprobe = new Softprobe();\n")
	buf.WriteString("    const session = await softprobe.startSession({ mode: 'replay' });\n")
	buf.WriteString("    close = () => session.close();\n")
	fmt.Fprintf(&buf, "    await session.loadCaseFromFile(path.join(__dirname, %q));\n", filepath.ToSlash(rel))
	emitTSRules(&buf, rules, "    ")
	buf.WriteString("    // TODO: exercise the system under test and assert.\n")
	buf.WriteString("  });\n")
	buf.WriteString("});\n")
	return buf.String(), nil
}

func emitPytestTestFile(casePath, outPath string, doc caseDocument, rules []jestSessionRule) (string, error) {
	rel, err := relFromOut(casePath, outPath)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	writeGenHeader(&buf, "# ")
	buf.WriteString("import os\n")
	buf.WriteString("import pytest\n")
	buf.WriteString("from softprobe import Softprobe\n\n")
	buf.WriteString("HERE = os.path.dirname(os.path.abspath(__file__))\n\n")
	buf.WriteString("@pytest.mark.asyncio\n")
	fmt.Fprintf(&buf, "async def %s() -> None:\n", pytestFuncNameFor(doc))
	buf.WriteString("    softprobe = Softprobe()\n")
	buf.WriteString("    session = await softprobe.start_session(mode='replay')\n")
	buf.WriteString("    try:\n")
	fmt.Fprintf(&buf, "        await session.load_case_from_file(os.path.join(HERE, %q))\n", filepath.ToSlash(rel))
	emitPythonRules(&buf, rules, "        ")
	buf.WriteString("        # TODO: exercise the system under test and assert.\n")
	buf.WriteString("    finally:\n")
	buf.WriteString("        await session.close()\n")
	return buf.String(), nil
}

func emitJUnitTestFile(casePath, outPath string, doc caseDocument, rules []jestSessionRule) (string, error) {
	rel, err := relFromOut(casePath, outPath)
	if err != nil {
		return "", err
	}
	base := strings.TrimSuffix(filepath.Base(outPath), filepath.Ext(outPath))
	className := sanitizeJavaIdentifier(base)

	var buf bytes.Buffer
	writeGenHeader(&buf, "// ")
	buf.WriteString("import java.nio.file.Path;\n")
	buf.WriteString("import java.nio.file.Paths;\n")
	buf.WriteString("import org.junit.jupiter.api.AfterEach;\n")
	buf.WriteString("import org.junit.jupiter.api.Test;\n")
	buf.WriteString("import io.softprobe.Softprobe;\n")
	buf.WriteString("import io.softprobe.ReplaySession;\n\n")
	fmt.Fprintf(&buf, "class %s {\n", className)
	buf.WriteString("  private ReplaySession session;\n\n")
	buf.WriteString("  @AfterEach\n")
	buf.WriteString("  void tearDown() throws Exception { if (session != null) session.close(); }\n\n")
	buf.WriteString("  @Test\n")
	fmt.Fprintf(&buf, "  void %s() throws Exception {\n", javaTestMethodFor(doc))
	buf.WriteString("    Softprobe softprobe = new Softprobe();\n")
	buf.WriteString("    session = softprobe.startSession(\"replay\");\n")
	fmt.Fprintf(&buf, "    Path casePath = Paths.get(System.getProperty(\"user.dir\"), %q);\n", filepath.ToSlash(rel))
	buf.WriteString("    session.loadCaseFromFile(casePath);\n")
	emitJavaRules(&buf, rules, "    ")
	buf.WriteString("    // TODO: exercise the system under test and assert.\n")
	buf.WriteString("  }\n")
	buf.WriteString("}\n")
	return buf.String(), nil
}

func writeGenHeader(buf *bytes.Buffer, commentPrefix string) {
	buf.WriteString(commentPrefix)
	buf.WriteString("Generated by `softprobe generate test` — do not edit by hand.\n")
	buf.WriteString(commentPrefix)
	buf.WriteString("Regenerate after every capture refresh.\n\n")
}

func relFromOut(casePath, outPath string) (string, error) {
	absCase, err := filepath.Abs(casePath)
	if err != nil {
		return "", err
	}
	absOut, err := filepath.Abs(outPath)
	if err != nil {
		return "", err
	}
	return filepath.Rel(filepath.Dir(absOut), absCase)
}

func describeNameFor(doc caseDocument) string {
	if doc.Suite != "" {
		return doc.Suite
	}
	if doc.CaseID != "" {
		return doc.CaseID
	}
	return "softprobe replay"
}

func itNameFor(doc caseDocument) string {
	if doc.CaseID != "" {
		return "replays " + doc.CaseID
	}
	return "replays captured case"
}

func pytestFuncNameFor(doc caseDocument) string {
	name := strings.ReplaceAll(strings.ToLower(doc.CaseID), "-", "_")
	name = strings.ReplaceAll(name, ".", "_")
	name = sanitizePythonIdentifier(name)
	if name == "" {
		name = "replays_captured_case"
	}
	return "test_" + name
}

func javaTestMethodFor(doc caseDocument) string {
	name := sanitizeJavaIdentifier(strings.ReplaceAll(doc.CaseID, ".", "_"))
	if name == "" {
		name = "ReplaysCapturedCase"
	}
	if r := rune(name[0]); r >= 'A' && r <= 'Z' {
		name = strings.ToLower(name[:1]) + name[1:]
	}
	return "replays" + strings.ToUpper(name[:1]) + name[1:]
}

func sanitizePythonIdentifier(s string) string {
	var b strings.Builder
	for i, r := range s {
		switch {
		case r == '_':
			b.WriteRune(r)
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9' && i > 0:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func sanitizeJavaIdentifier(s string) string {
	var b strings.Builder
	capitalize := true
	for _, r := range s {
		switch {
		case r == '_' || r == '-' || r == '.' || r == ' ':
			capitalize = true
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'):
			if capitalize && r >= 'a' && r <= 'z' {
				b.WriteRune(r - ('a' - 'A'))
			} else {
				b.WriteRune(r)
			}
			capitalize = false
		}
	}
	return b.String()
}

func emitTSRules(buf *bytes.Buffer, rules []jestSessionRule, indent string) {
	for i, rule := range rules {
		fmt.Fprintf(buf, "%sconst hit%d = session.findInCase({\n", indent, i)
		if rule.Direction != "" {
			fmt.Fprintf(buf, "%s  direction: %q,\n", indent, rule.Direction)
		}
		if rule.Method != "" {
			fmt.Fprintf(buf, "%s  method: %q,\n", indent, rule.Method)
		}
		if rule.Path != "" {
			fmt.Fprintf(buf, "%s  path: %q,\n", indent, rule.Path)
		}
		fmt.Fprintf(buf, "%s});\n", indent)
		fmt.Fprintf(buf, "%sawait session.mockOutbound({ ", indent)
		if rule.Method != "" {
			fmt.Fprintf(buf, "method: %q, ", rule.Method)
		}
		if rule.Path != "" {
			fmt.Fprintf(buf, "path: %q, ", rule.Path)
		}
		fmt.Fprintf(buf, "response: hit%d.response });\n", i)
	}
}

func emitPythonRules(buf *bytes.Buffer, rules []jestSessionRule, indent string) {
	for i, rule := range rules {
		fmt.Fprintf(buf, "%shit_%d = session.find_in_case(", indent, i)
		parts := []string{}
		if rule.Direction != "" {
			parts = append(parts, fmt.Sprintf("direction=%q", rule.Direction))
		}
		if rule.Method != "" {
			parts = append(parts, fmt.Sprintf("method=%q", rule.Method))
		}
		if rule.Path != "" {
			parts = append(parts, fmt.Sprintf("path=%q", rule.Path))
		}
		buf.WriteString(strings.Join(parts, ", "))
		buf.WriteString(")\n")
		fmt.Fprintf(buf, "%sawait session.mock_outbound(", indent)
		mparts := []string{}
		if rule.Method != "" {
			mparts = append(mparts, fmt.Sprintf("method=%q", rule.Method))
		}
		if rule.Path != "" {
			mparts = append(mparts, fmt.Sprintf("path=%q", rule.Path))
		}
		mparts = append(mparts, fmt.Sprintf("response=hit_%d.response", i))
		buf.WriteString(strings.Join(mparts, ", "))
		buf.WriteString(")\n")
	}
}

func emitJavaRules(buf *bytes.Buffer, rules []jestSessionRule, indent string) {
	for i, rule := range rules {
		fmt.Fprintf(buf, "%svar hit%d = session.findInCase()", indent, i)
		if rule.Direction != "" {
			fmt.Fprintf(buf, ".direction(%q)", rule.Direction)
		}
		if rule.Method != "" {
			fmt.Fprintf(buf, ".method(%q)", rule.Method)
		}
		if rule.Path != "" {
			fmt.Fprintf(buf, ".path(%q)", rule.Path)
		}
		buf.WriteString(".find();\n")
		fmt.Fprintf(buf, "%ssession.mockOutbound()", indent)
		if rule.Method != "" {
			fmt.Fprintf(buf, ".method(%q)", rule.Method)
		}
		if rule.Path != "" {
			fmt.Fprintf(buf, ".path(%q)", rule.Path)
		}
		fmt.Fprintf(buf, ".response(hit%d.response()).apply();\n", i)
	}
}
