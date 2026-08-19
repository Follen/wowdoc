package validator

import (
	"bufio"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const (
	diagnosticMissingFile     = "load_file_missing"
	diagnosticUnreadableFile  = "load_file_unreadable"
	diagnosticPathEscape      = "load_path_escape"
	diagnosticUnsupportedType = "unsupported_load_type"
	diagnosticXMLParse        = "xml_parse_failed"
	diagnosticDuplicate       = "duplicate_load_reference"
	diagnosticCycle           = "circular_load_reference"
)

// BuildClosure parses the ordered Lua/XML load closure rooted at toc. The toc
// argument may be relative to root, or an absolute path contained by root.
func BuildClosure(root, toc string) (Closure, error) {
	result := Closure{
		Files:       []LoadFile{},
		Diagnostics: []Diagnostic{},
		Contents:    map[string][]byte{},
	}

	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return result, fmt.Errorf("resolve AddOn root: %w", err)
	}
	rootInfo, err := os.Stat(rootAbs)
	if err != nil {
		return result, fmt.Errorf("open AddOn root: %w", err)
	}
	if !rootInfo.IsDir() {
		return result, fmt.Errorf("AddOn root is not a directory: %s", root)
	}
	rootReal, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return result, fmt.Errorf("resolve AddOn root links: %w", err)
	}
	rootReal, err = filepath.Abs(rootReal)
	if err != nil {
		return result, fmt.Errorf("resolve real AddOn root: %w", err)
	}

	b := closureBuilder{
		rootAbs:  rootAbs,
		rootReal: rootReal,
		result:   result,
		files:    map[string]int{},
		active:   map[string]bool{},
	}

	tocPath, resolveErr := b.resolve("", toc, true)
	if resolveErr != nil {
		b.addResolveDiagnostic(LoadSource{File: normalizeDisplayPath(toc), Kind: "toc"}, toc, resolveErr)
		return b.result, nil
	}
	b.result.TOC = tocPath.rel
	data, err := os.ReadFile(tocPath.disk)
	if err != nil {
		code := diagnosticUnreadableFile
		if errors.Is(err, os.ErrNotExist) {
			code = diagnosticMissingFile
		}
		b.diagnostic("error", code, tocPath.rel, 0, 0, "cannot read TOC file", Evidence{
			"path":  tocPath.rel,
			"error": err.Error(),
		})
		return b.result, nil
	}
	b.parseTOC(data)
	return b.result, nil
}

type closureBuilder struct {
	rootAbs  string
	rootReal string
	result   Closure
	files    map[string]int
	active   map[string]bool
}

type resolvedLoadPath struct {
	rel  string
	disk string
}

type resolveKind int

const (
	resolveEscape resolveKind = iota + 1
	resolveMissing
	resolveUnreadable
)

type resolveError struct {
	kind resolveKind
	path string
	err  error
}

func (e *resolveError) Error() string {
	if e.err == nil {
		return e.path
	}
	return e.err.Error()
}

func (b *closureBuilder) parseTOC(data []byte) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	line := 0
	for scanner.Scan() {
		line++
		value := strings.TrimSpace(scanner.Text())
		if line == 1 {
			value = strings.TrimPrefix(value, "\ufeff")
		}
		if value == "" {
			continue
		}
		if strings.HasPrefix(value, "##") {
			key, metadataValue, ok := strings.Cut(strings.TrimSpace(strings.TrimPrefix(value, "##")), ":")
			if ok && strings.EqualFold(strings.TrimSpace(key), "Interface") {
				b.result.Interface = strings.TrimSpace(metadataValue)
			}
			continue
		}
		if strings.HasPrefix(value, "#") {
			continue
		}
		b.load(value, LoadSource{File: b.result.TOC, Line: line, Kind: "toc"})
	}
	if err := scanner.Err(); err != nil {
		b.diagnostic("error", diagnosticUnreadableFile, b.result.TOC, line, 0, "cannot read TOC contents", Evidence{
			"error": err.Error(),
		})
	}
}

func (b *closureBuilder) load(reference string, source LoadSource) {
	resolved, err := b.resolve(source.File, reference, false)
	if err != nil {
		b.addResolveDiagnostic(source, reference, err)
		return
	}

	fileType := strings.TrimPrefix(strings.ToLower(path.Ext(resolved.rel)), ".")
	if fileType != "lua" && fileType != "xml" {
		b.diagnostic("error", diagnosticUnsupportedType, source.File, source.Line, 0,
			fmt.Sprintf("unsupported AddOn load file type %q", path.Ext(resolved.rel)), Evidence{
				"reference": reference,
				"resolved":  resolved.rel,
			})
		return
	}

	key := strings.ToLower(resolved.rel)
	if index, ok := b.files[key]; ok {
		b.result.Files[index].LoadedBy = append(b.result.Files[index].LoadedBy, source)
		if b.active[key] {
			b.diagnostic("warning", diagnosticCycle, source.File, source.Line, 0,
				fmt.Sprintf("load cycle references %s", resolved.rel), Evidence{
					"reference": reference,
					"target":    resolved.rel,
				})
		} else {
			b.diagnostic("warning", diagnosticDuplicate, source.File, source.Line, 0,
				fmt.Sprintf("duplicate load reference to %s", resolved.rel), Evidence{
					"reference":     reference,
					"target":        resolved.rel,
					"firstLoadedBy": b.result.Files[index].LoadedBy[0],
				})
		}
		return
	}

	data, readErr := os.ReadFile(resolved.disk)
	if readErr != nil {
		code := diagnosticUnreadableFile
		if errors.Is(readErr, os.ErrNotExist) {
			code = diagnosticMissingFile
		}
		b.diagnostic("error", code, source.File, source.Line, 0,
			fmt.Sprintf("cannot read referenced file %s", resolved.rel), Evidence{
				"reference": reference,
				"target":    resolved.rel,
				"error":     readErr.Error(),
			})
		return
	}

	index := len(b.result.Files)
	b.files[key] = index
	b.result.Files = append(b.result.Files, LoadFile{
		Path:      resolved.rel,
		Type:      fileType,
		LoadedBy:  []LoadSource{source},
		LoadOrder: index,
	})
	b.result.Contents[resolved.rel] = data
	if fileType != "xml" {
		return
	}

	b.active[key] = true
	b.parseXML(resolved.rel, data)
	delete(b.active, key)
}

func (b *closureBuilder) parseXML(file string, data []byte) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	for {
		startOffset := decoder.InputOffset()
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return
		}
		if err != nil {
			line, column := bytePosition(data, decoder.InputOffset())
			var syntaxErr *xml.SyntaxError
			if errors.As(err, &syntaxErr) {
				line = syntaxErr.Line
			}
			b.diagnostic("error", diagnosticXMLParse, file, line, column,
				fmt.Sprintf("cannot parse XML: %v", err), Evidence{"error": err.Error()})
			return
		}

		start, ok := token.(xml.StartElement)
		if !ok || (!strings.EqualFold(start.Name.Local, "Script") && !strings.EqualFold(start.Name.Local, "Include")) {
			continue
		}
		reference := ""
		for _, attribute := range start.Attr {
			if strings.EqualFold(attribute.Name.Local, "file") {
				reference = strings.TrimSpace(attribute.Value)
				break
			}
		}
		if reference == "" {
			continue
		}
		line, _ := bytePosition(data, startOffset)
		b.load(reference, LoadSource{
			File: file,
			Line: line,
			Kind: strings.ToLower(start.Name.Local),
		})
	}
}

func (b *closureBuilder) resolve(fromFile, reference string, allowAbsolute bool) (resolvedLoadPath, error) {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return resolvedLoadPath{}, &resolveError{kind: resolveMissing, path: reference, err: os.ErrNotExist}
	}

	nativeReference := filepath.FromSlash(strings.ReplaceAll(reference, "\\", "/"))
	isAbsolute := filepath.IsAbs(nativeReference) || hasWindowsVolume(reference)
	var candidate string
	if isAbsolute {
		if !allowAbsolute || !filepath.IsAbs(nativeReference) {
			return resolvedLoadPath{}, &resolveError{kind: resolveEscape, path: reference}
		}
		candidate = filepath.Clean(nativeReference)
	} else {
		base := path.Dir(fromFile)
		cleanRelative := path.Clean(path.Join(base, strings.ReplaceAll(reference, "\\", "/")))
		if cleanRelative == ".." || strings.HasPrefix(cleanRelative, "../") || strings.HasPrefix(cleanRelative, "/") {
			return resolvedLoadPath{}, &resolveError{kind: resolveEscape, path: cleanRelative}
		}
		candidate = filepath.Join(b.rootAbs, filepath.FromSlash(cleanRelative))
	}

	rel, err := filepath.Rel(b.rootAbs, candidate)
	if err != nil || escapesRoot(rel) {
		return resolvedLoadPath{}, &resolveError{kind: resolveEscape, path: reference, err: err}
	}
	rel = filepath.Clean(rel)

	actual, findErr := b.findCaseInsensitive(rel)
	if findErr != nil {
		var nestedResolveErr *resolveError
		if errors.As(findErr, &nestedResolveErr) {
			return resolvedLoadPath{}, nestedResolveErr
		}
		if errors.Is(findErr, os.ErrNotExist) {
			return resolvedLoadPath{}, &resolveError{kind: resolveMissing, path: normalizeDisplayPath(rel), err: findErr}
		}
		return resolvedLoadPath{}, &resolveError{kind: resolveUnreadable, path: normalizeDisplayPath(rel), err: findErr}
	}
	real, err := filepath.EvalSymlinks(actual)
	if err != nil {
		kind := resolveUnreadable
		if errors.Is(err, os.ErrNotExist) {
			kind = resolveMissing
		}
		return resolvedLoadPath{}, &resolveError{kind: kind, path: normalizeDisplayPath(rel), err: err}
	}
	real, err = filepath.Abs(real)
	if err != nil {
		return resolvedLoadPath{}, &resolveError{kind: resolveUnreadable, path: normalizeDisplayPath(rel), err: err}
	}
	realRel, err := filepath.Rel(b.rootReal, real)
	if err != nil || escapesRoot(realRel) {
		return resolvedLoadPath{}, &resolveError{kind: resolveEscape, path: normalizeDisplayPath(rel), err: err}
	}

	actualRel, err := filepath.Rel(b.rootAbs, actual)
	if err != nil || escapesRoot(actualRel) {
		return resolvedLoadPath{}, &resolveError{kind: resolveEscape, path: normalizeDisplayPath(rel), err: err}
	}
	return resolvedLoadPath{rel: normalizeDisplayPath(actualRel), disk: actual}, nil
}

func (b *closureBuilder) findCaseInsensitive(rel string) (string, error) {
	current := b.rootAbs
	for _, component := range strings.FieldsFunc(filepath.Clean(rel), func(r rune) bool {
		return r == '/' || r == '\\'
	}) {
		exact := filepath.Join(current, component)
		if _, err := os.Lstat(exact); err == nil {
			current = exact
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		} else {
			entries, readErr := os.ReadDir(current)
			if readErr != nil {
				return "", readErr
			}
			matched := ""
			for _, entry := range entries {
				if strings.EqualFold(entry.Name(), component) {
					matched = entry.Name()
					break
				}
			}
			if matched == "" {
				return "", &os.PathError{Op: "open", Path: exact, Err: os.ErrNotExist}
			}
			current = filepath.Join(current, matched)
		}

		real, evalErr := filepath.EvalSymlinks(current)
		if evalErr != nil {
			return "", evalErr
		}
		realRel, relErr := filepath.Rel(b.rootReal, real)
		if relErr != nil || escapesRoot(realRel) {
			return "", &resolveError{kind: resolveEscape, path: normalizeDisplayPath(rel), err: relErr}
		}
	}
	return current, nil
}

func (b *closureBuilder) addResolveDiagnostic(source LoadSource, reference string, err error) {
	var resolveErr *resolveError
	if !errors.As(err, &resolveErr) {
		b.diagnostic("error", diagnosticUnreadableFile, source.File, source.Line, 0,
			"cannot resolve load path", Evidence{"reference": reference, "error": err.Error()})
		return
	}
	code := diagnosticUnreadableFile
	message := "cannot resolve referenced file"
	switch resolveErr.kind {
	case resolveEscape:
		code = diagnosticPathEscape
		message = "load path escapes the AddOn root"
	case resolveMissing:
		code = diagnosticMissingFile
		message = "referenced file does not exist"
	}
	evidence := Evidence{"reference": reference, "resolved": resolveErr.path}
	if resolveErr.err != nil {
		evidence["error"] = resolveErr.err.Error()
	}
	b.diagnostic("error", code, source.File, source.Line, 0, message, evidence)
}

func (b *closureBuilder) diagnostic(severity, code, file string, line, column int, message string, evidence Evidence) {
	if evidence == nil {
		evidence = Evidence{}
	}
	b.result.Diagnostics = append(b.result.Diagnostics, Diagnostic{
		Severity: severity,
		Code:     code,
		File:     normalizeDisplayPath(file),
		Line:     line,
		Column:   column,
		Message:  message,
		Evidence: evidence,
	})
}

func escapesRoot(rel string) bool {
	return rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel)
}

func hasWindowsVolume(value string) bool {
	value = strings.ReplaceAll(value, "\\", "/")
	hasDrive := len(value) >= 2 && ((value[0] >= 'a' && value[0] <= 'z') || (value[0] >= 'A' && value[0] <= 'Z')) && value[1] == ':'
	return hasDrive || strings.HasPrefix(value, "//")
}

func normalizeDisplayPath(value string) string {
	return filepath.ToSlash(filepath.Clean(value))
}

func bytePosition(data []byte, offset int64) (line, column int) {
	if offset < 0 {
		offset = 0
	}
	if offset > int64(len(data)) {
		offset = int64(len(data))
	}
	prefix := data[:offset]
	line = bytes.Count(prefix, []byte{'\n'}) + 1
	lastNewline := bytes.LastIndexByte(prefix, '\n')
	columnBytes := prefix[lastNewline+1:]
	column = utf8.RuneCount(columnBytes) + 1
	return line, column
}
