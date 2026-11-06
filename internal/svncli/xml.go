package svncli

import (
	"encoding/xml"
	"errors"
	"fmt"
	"strconv"
	"time"
)

// StatusEntry is one working copy item reported by "svn status --xml".
//
// Path is kept exactly as svn prints it, backslash separators included, so it
// can be handed back to svn or joined to a working copy root without a lossy
// round trip.
type StatusEntry struct {
	Path     string
	Item     string
	Props    string
	Revision string
	Author   string
	Date     time.Time
}

// Conflicted reports whether the item is in conflict. It is the only status
// that blocks a commit until the user resolves it, so callers single it out.
func (e StatusEntry) Conflicted() bool {
	return e.Item == "conflicted"
}

// Info is the working copy view of a single path, as reported by
// "svn info --xml".
//
// Revision is that path's BASE revision, not the repository HEAD: Subversion
// working copies are mixed revision, so a single "current revision" does not
// exist.
type Info struct {
	Path                string
	Kind                string
	Revision            string
	URL                 string
	RelativeURL         string
	RepoRoot            string
	RepoUUID            string
	WCRootAbsPath       string
	WCFormat            string
	LastChangedRevision string
	LastChangedAuthor   string
	LastChangedDate     time.Time
}

// LogEntry is one revision of "svn log --xml -v" output.
type LogEntry struct {
	Revision int64
	Author   string
	Date     time.Time
	Message  string
	Paths    []PathChange
}

// PathChange is a path touched by a LogEntry. TextMods and PropMods separate a
// content change from a property-only change, which the UI needs so it does not
// claim a file diff exists when only properties moved.
type PathChange struct {
	Path     string
	Action   string
	Kind     string
	TextMods bool
	PropMods bool
}

// BlameFile is one target of "svn blame --xml". A single run can emit several
// targets, for example a file and its conflicted ".mine" sibling.
type BlameFile struct {
	TargetPath string
	Lines      []BlameLine
}

// BlameLine is one annotated line. Revision and Author stay empty when svn omits
// the commit element, which happens for lines not touched by the queried range.
type BlameLine struct {
	LineNumber int
	Revision   string
	Author     string
	Date       time.Time
}

// ParseStatus decodes "svn status --xml" output. Entries grouped in changelists
// are returned in the same flat slice as the rest: callers need the state of
// each path first and the grouping only afterwards.
func ParseStatus(data []byte) ([]StatusEntry, error) {
	var doc statusXML
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse svn status xml: %w", err)
	}
	var entries []StatusEntry
	for _, target := range doc.Targets {
		if len(target.Entries) > 0 {
			parsed, err := parseStatusEntries(target.Entries)
			if err != nil {
				return nil, err
			}
			entries = append(entries, parsed...)
		}
		for _, list := range target.Changelists {
			parsed, err := parseStatusEntries(list.Entries)
			if err != nil {
				return nil, err
			}
			entries = append(entries, parsed...)
		}
	}
	return entries, nil
}

func parseStatusEntries(raw []statusEntryXML) ([]StatusEntry, error) {
	entries := make([]StatusEntry, 0, len(raw))
	for _, item := range raw {
		entry := StatusEntry{
			Path:     item.Path,
			Item:     item.WCStatus.Item,
			Props:    item.WCStatus.Props,
			Revision: item.WCStatus.Revision,
		}
		if item.WCStatus.Commit != nil {
			date, err := parseTime(item.WCStatus.Commit.Date)
			if err != nil {
				return nil, fmt.Errorf("parse svn status date for %q: %w", entry.Path, err)
			}
			entry.Author = item.WCStatus.Commit.Author
			entry.Date = date
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// ParseInfo decodes "svn info --xml" output and reports the first entry. "svn
// info" accepts several targets, but the contract describes one path, so extra
// entries are ignored instead of being merged into a misleading summary.
func ParseInfo(data []byte) (*Info, error) {
	var doc infoXML
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse svn info xml: %w", err)
	}
	if len(doc.Entries) == 0 {
		return nil, errors.New("parse svn info xml: no entry element")
	}
	raw := doc.Entries[0]
	info := &Info{
		Path:          raw.Path,
		Kind:          raw.Kind,
		Revision:      raw.Revision,
		URL:           raw.URL,
		RelativeURL:   raw.RelURL,
		RepoRoot:      raw.Repository.Root,
		RepoUUID:      raw.Repository.UUID,
		WCRootAbsPath: raw.WCInfo.RootAbsPath,
		WCFormat:      raw.WCInfo.Format,
	}
	if raw.Commit != nil {
		date, err := parseTime(raw.Commit.Date)
		if err != nil {
			return nil, fmt.Errorf("parse svn info date for %q: %w", info.Path, err)
		}
		info.LastChangedRevision = raw.Commit.Revision
		info.LastChangedAuthor = raw.Commit.Author
		info.LastChangedDate = date
	}
	return info, nil
}

// ParseLog decodes "svn log --xml -v" output in svn's order, newest revision
// first. Author and Message stay empty for anonymous commits and empty messages,
// and revisions become numbers so callers can compare ranges.
func ParseLog(data []byte) ([]LogEntry, error) {
	var doc logXML
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse svn log xml: %w", err)
	}
	entries := make([]LogEntry, 0, len(doc.Entries))
	for _, raw := range doc.Entries {
		revision, err := strconv.ParseInt(raw.Revision, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse svn log revision %q: %w", raw.Revision, err)
		}
		date, err := parseTime(raw.Date)
		if err != nil {
			return nil, fmt.Errorf("parse svn log date for r%d: %w", revision, err)
		}
		entry := LogEntry{
			Revision: revision,
			Author:   raw.Author,
			Date:     date,
			Message:  raw.Message,
		}
		if len(raw.Paths.Paths) > 0 {
			entry.Paths = make([]PathChange, 0, len(raw.Paths.Paths))
		}
		for _, changed := range raw.Paths.Paths {
			textMods, err := parseBoolAttr(changed.TextMods)
			if err != nil {
				return nil, fmt.Errorf("parse svn log text-mods for %q: %w", changed.Path, err)
			}
			propMods, err := parseBoolAttr(changed.PropMods)
			if err != nil {
				return nil, fmt.Errorf("parse svn log prop-mods for %q: %w", changed.Path, err)
			}
			entry.Paths = append(entry.Paths, PathChange{
				Path:     changed.Path,
				Action:   changed.Action,
				Kind:     changed.Kind,
				TextMods: textMods,
				PropMods: propMods,
			})
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// ParseBlame decodes "svn blame --xml" output. Targets without lines are kept so
// the caller can still report which files were annotated.
func ParseBlame(data []byte) ([]BlameFile, error) {
	var doc blameXML
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse svn blame xml: %w", err)
	}
	files := make([]BlameFile, 0, len(doc.Targets))
	for _, target := range doc.Targets {
		file := BlameFile{TargetPath: target.Path}
		if len(target.Entries) > 0 {
			file.Lines = make([]BlameLine, 0, len(target.Entries))
		}
		for _, raw := range target.Entries {
			number, err := strconv.Atoi(raw.LineNumber)
			if err != nil {
				return nil, fmt.Errorf("parse svn blame line number %q in %q: %w", raw.LineNumber, target.Path, err)
			}
			line := BlameLine{LineNumber: number}
			if raw.Commit != nil {
				date, err := parseTime(raw.Commit.Date)
				if err != nil {
					return nil, fmt.Errorf("parse svn blame date for %q line %d: %w", target.Path, number, err)
				}
				line.Revision = raw.Commit.Revision
				line.Author = raw.Commit.Author
				line.Date = date
			}
			file.Lines = append(file.Lines, line)
		}
		files = append(files, file)
	}
	return files, nil
}

// parseTime accepts svn's timestamps as they are and treats a missing or empty
// value as the zero time: unversioned entries and untouched blame lines simply
// carry no date, and that is not an error worth failing a whole command for.
func parseTime(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	date, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse timestamp %q: %w", value, err)
	}
	return date, nil
}

// parseBoolAttr reads svn's "true"/"false" attributes. An absent attribute means
// "not reported", which for text-mods and prop-mods is the same as false.
func parseBoolAttr(value string) (bool, error) {
	if value == "" {
		return false, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("parse boolean %q: %w", value, err)
	}
	return parsed, nil
}

// The wire structures below mirror svn's schema loosely on purpose: encoding/xml
// ignores elements and attributes it has no field for, so an svn release that
// adds optional metadata keeps parsing instead of breaking the whole view.
// Optional children are pointers, so "absent" is distinguishable from "empty".

type statusXML struct {
	Targets []statusTargetXML `xml:"target"`
}

type statusTargetXML struct {
	Path        string                `xml:"path,attr"`
	Entries     []statusEntryXML      `xml:"entry"`
	Changelists []statusChangelistXML `xml:"changelist"`
}

// statusChangelistXML collects entries nested one level deeper; without it the
// paths of a changelist would silently vanish from status output.
type statusChangelistXML struct {
	Name    string           `xml:"name,attr"`
	Entries []statusEntryXML `xml:"entry"`
}

type statusEntryXML struct {
	Path     string      `xml:"path,attr"`
	WCStatus statusWCXML `xml:"wc-status"`
}

type statusWCXML struct {
	Item     string           `xml:"item,attr"`
	Props    string           `xml:"props,attr"`
	Revision string           `xml:"revision,attr"`
	Commit   *statusCommitXML `xml:"commit"`
}

// statusCommitXML is nil for unversioned and just-added entries, so the pointer
// is what marks "no commit information" rather than an empty author.
type statusCommitXML struct {
	Author string `xml:"author"`
	Date   string `xml:"date"`
}

type infoXML struct {
	Entries []infoEntryXML `xml:"entry"`
}

type infoEntryXML struct {
	Kind       string            `xml:"kind,attr"`
	Path       string            `xml:"path,attr"`
	Revision   string            `xml:"revision,attr"`
	URL        string            `xml:"url"`
	RelURL     string            `xml:"relative-url"`
	Repository infoRepositoryXML `xml:"repository"`
	WCInfo     infoWCXML         `xml:"wc-info"`
	Commit     *infoCommitXML    `xml:"commit"`
}

type infoRepositoryXML struct {
	Root string `xml:"root"`
	UUID string `xml:"uuid"`
}

type infoWCXML struct {
	RootAbsPath string `xml:"wcroot-abspath"`
	Format      string `xml:"wc-format"`
}

type infoCommitXML struct {
	Revision string `xml:"revision,attr"`
	Author   string `xml:"author"`
	Date     string `xml:"date"`
}

type logXML struct {
	Entries []logEntryXML `xml:"logentry"`
}

type logEntryXML struct {
	Revision string      `xml:"revision,attr"`
	Author   string      `xml:"author"`
	Date     string      `xml:"date"`
	Paths    logPathsXML `xml:"paths"`
	Message  string      `xml:"msg"`
}

type logPathsXML struct {
	Paths []logPathXML `xml:"path"`
}

type logPathXML struct {
	TextMods string `xml:"text-mods,attr"`
	Kind     string `xml:"kind,attr"`
	Action   string `xml:"action,attr"`
	PropMods string `xml:"prop-mods,attr"`
	Path     string `xml:",chardata"`
}

type blameXML struct {
	Targets []blameTargetXML `xml:"target"`
}

type blameTargetXML struct {
	Path    string          `xml:"path,attr"`
	Entries []blameEntryXML `xml:"entry"`
}

type blameEntryXML struct {
	LineNumber string          `xml:"line-number,attr"`
	Commit     *blameCommitXML `xml:"commit"`
}

type blameCommitXML struct {
	Revision string `xml:"revision,attr"`
	Author   string `xml:"author"`
	Date     string `xml:"date"`
}

// ListEntry is one entry of "svn ls --xml".
//
// Name is the entry's own name, not a path: `svn ls -R` prints nested paths as
// chardata instead, so callers that need full paths must ask for a
// non-recursive listing per directory or join them themselves.
type ListEntry struct {
	Name       string
	Kind       string // "file" or "dir"
	Size       int64
	Revision   string
	CommitAuth string
	CommitDate time.Time
}

type listsXML struct {
	Lists []listXML `xml:"list"`
}

type listXML struct {
	Path    string         `xml:"path,attr"`
	Entries []listEntryXML `xml:"entry"`
}

type listEntryXML struct {
	Kind   string         `xml:"kind,attr"`
	Name   string         `xml:"name"`
	Size   string         `xml:"size"`
	Commit *listCommitXML `xml:"commit"`
}

type listCommitXML struct {
	Revision string `xml:"revision,attr"`
	Author   string `xml:"author"`
	Date     string `xml:"date"`
}

// ParseList parses "svn ls --xml" output. Recursive listings flatten every
// <list> into one slice, and each entry keeps the name svn printed, so a
// recursive call yields the relative path in Name.
func ParseList(data []byte) ([]ListEntry, error) {
	var doc listsXML
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse svn ls xml: %w", err)
	}
	var out []ListEntry
	for _, list := range doc.Lists {
		for _, raw := range list.Entries {
			entry := ListEntry{Name: raw.Name, Kind: raw.Kind}
			if entry.Kind == "" {
				entry.Kind = "file"
			}
			if raw.Size != "" {
				if n, err := strconv.ParseInt(raw.Size, 10, 64); err == nil {
					entry.Size = n
				}
			}
			if raw.Commit != nil {
				entry.Revision = raw.Commit.Revision
				entry.CommitAuth = raw.Commit.Author
				date, err := parseTime(raw.Commit.Date)
				if err != nil {
					return nil, fmt.Errorf("parse svn ls date for %q: %w", raw.Name, err)
				}
				entry.CommitDate = date
			}
			out = append(out, entry)
		}
	}
	return out, nil
}
