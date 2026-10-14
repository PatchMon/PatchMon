package packages

import "strings"

const (
	apkTokenInitialDigit = iota
	apkTokenDigit
	apkTokenLetter
	apkTokenSuffix
	apkTokenSuffixNo
	apkTokenCommitHash
	apkTokenRevisionNo
	apkTokenEnd
	apkTokenInvalid
)

const (
	apkSuffixInvalid = iota
	apkSuffixAlpha
	apkSuffixBeta
	apkSuffixPre
	apkSuffixRC
	apkSuffixNone
	apkSuffixCVS
	apkSuffixSVN
	apkSuffixGit
	apkSuffixHg
	apkSuffixP
)

var apkSuffixValues = map[string]int{
	"alpha": apkSuffixAlpha, "beta": apkSuffixBeta, "pre": apkSuffixPre, "rc": apkSuffixRC,
	"cvs": apkSuffixCVS, "svn": apkSuffixSVN, "git": apkSuffixGit, "hg": apkSuffixHg, "p": apkSuffixP,
}

type apkVersionToken struct {
	kind   int
	suffix int
	number uint64
	value  string
}

type apkVersionLexer struct {
	s string
	t apkVersionToken
}

func (l *apkVersionLexer) digits() {
	i := 0
	var n uint64
	for i < len(l.s) && l.s[i] >= '0' && l.s[i] <= '9' {
		n = n*10 + uint64(l.s[i]-'0')
		i++
	}
	l.t.number = n
	l.t.value = l.s[:i]
	l.s = l.s[i:]
	if i == 0 {
		l.t.kind = apkTokenInvalid
	}
}

func (l *apkVersionLexer) span(ok func(byte) bool) string {
	i := 0
	for i < len(l.s) && ok(l.s[i]) {
		i++
	}
	v := l.s[:i]
	l.s = l.s[i:]
	return v
}

func (l *apkVersionLexer) next() {
	if l.s == "" {
		l.t.kind = apkTokenEnd
		return
	}
	c := l.s[0]
	switch {
	case c >= 'a' && c <= 'z':
		if l.t.kind > apkTokenDigit {
			l.t.kind = apkTokenInvalid
			return
		}
		l.t.kind = apkTokenLetter
		l.t.value = l.s[:1]
		l.s = l.s[1:]
	case c == '.':
		if l.t.kind > apkTokenDigit {
			l.t.kind = apkTokenInvalid
			return
		}
		l.s = l.s[1:]
		l.t.kind = apkTokenDigit
		l.digits()
	case c == '_':
		if l.t.kind > apkTokenSuffixNo {
			l.t.kind = apkTokenInvalid
			return
		}
		l.s = l.s[1:]
		l.t.value = l.span(func(b byte) bool { return b >= 'a' && b <= 'z' })
		l.t.suffix = apkSuffixValues[l.t.value]
		if l.t.suffix == apkSuffixInvalid {
			l.t.kind = apkTokenInvalid
			return
		}
		l.t.kind = apkTokenSuffix
	case c >= '0' && c <= '9':
		if l.t.kind != apkTokenSuffix {
			l.t.kind = apkTokenInvalid
			return
		}
		l.t.kind = apkTokenSuffixNo
		l.digits()
	case c == '~':
		if l.t.kind >= apkTokenCommitHash {
			l.t.kind = apkTokenInvalid
			return
		}
		l.s = l.s[1:]
		l.t.value = l.span(func(b byte) bool {
			return (b >= '0' && b <= '9') || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F')
		})
		if l.t.value == "" {
			l.t.kind = apkTokenInvalid
			return
		}
		l.t.kind = apkTokenCommitHash
	case c == '-':
		if l.t.kind >= apkTokenRevisionNo || !strings.HasPrefix(l.s, "-r") {
			l.t.kind = apkTokenInvalid
			return
		}
		l.s = l.s[2:]
		l.t.kind = apkTokenRevisionNo
		l.digits()
	default:
		l.t.kind = apkTokenInvalid
	}
}

func apkTokenCompare(a, b apkVersionToken, legacy bool) int {
	var x, y uint64
	switch a.kind {
	case apkTokenDigit:
		if strings.HasPrefix(a.value, "0") || strings.HasPrefix(b.value, "0") {
			if legacy {
				return apkLegacyLeadingZeroCompare(a.value, b.value)
			}
			return strings.Compare(a.value, b.value)
		}
		x, y = a.number, b.number
	case apkTokenInitialDigit, apkTokenSuffixNo, apkTokenRevisionNo:
		x, y = a.number, b.number
	case apkTokenLetter:
		x, y = uint64(a.value[0]), uint64(b.value[0])
	case apkTokenSuffix:
		x, y = uint64(a.suffix), uint64(b.suffix)
	default:
		return strings.Compare(a.value, b.value)
	}
	switch {
	case x < y:
		return -1
	case x > y:
		return 1
	}
	return 0
}

// apk-tools 2 reads a run of leading zeros as a negative number and the digits
// after it as the next component, so 0.012 sorts above 0.04 there and below it on 3.
func apkLegacyLeadingZeroCompare(a, b string) int {
	key := func(s string) (int, uint64, bool) {
		zeros := len(s) - len(strings.TrimLeft(s, "0"))
		var n uint64
		for _, c := range s[zeros:] {
			n = n*10 + uint64(c-'0')
		}
		if zeros == 0 {
			return int(min(n, 1<<31)), 0, false
		}
		return -zeros, n, zeros < len(s)
	}
	ka, ra, hasA := key(a)
	kb, rb, hasB := key(b)
	switch {
	case ka != kb:
		return cmpInt(ka, kb)
	case hasA != hasB:
		if hasA {
			return 1
		}
		return -1
	case ra < rb:
		return -1
	case ra > rb:
		return 1
	}
	return 0
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// CompareAPKVersions orders two Alpine package versions the way apk-tools 3 does,
// returning -1, 0 or 1.
func CompareAPKVersions(a, b string) int {
	return compareAPKVersions(a, b, false)
}

// CompareAPKVersionsLegacy orders versions the way apk-tools 2 does. The two only
// disagree on components with leading zeros, such as 0.012 against 0.04.
func CompareAPKVersionsLegacy(a, b string) int {
	return compareAPKVersions(a, b, true)
}

func apkVersionComparer(apkMajor int) func(a, b string) int {
	if apkMajor == 2 {
		return CompareAPKVersionsLegacy
	}
	return CompareAPKVersions
}

func apkVersionValid(v string) bool {
	l := apkVersionLexer{s: v, t: apkVersionToken{kind: apkTokenInitialDigit}}
	l.digits()
	for l.t.kind < apkTokenEnd {
		l.next()
	}
	return l.t.kind == apkTokenEnd
}

func compareAPKVersions(a, b string, legacy bool) int {
	la := apkVersionLexer{s: a, t: apkVersionToken{kind: apkTokenInitialDigit}}
	lb := apkVersionLexer{s: b, t: apkVersionToken{kind: apkTokenInitialDigit}}
	la.digits()
	lb.digits()
	for la.t.kind == lb.t.kind && la.t.kind < apkTokenEnd {
		if r := apkTokenCompare(la.t, lb.t, legacy); r != 0 {
			return r
		}
		la.next()
		lb.next()
	}
	ta, tb := la.t, lb.t
	if ta.kind == tb.kind {
		return 0
	}
	if ta.kind == apkTokenSuffix && ta.suffix < apkSuffixNone {
		return -1
	}
	if tb.kind == apkTokenSuffix && tb.suffix < apkSuffixNone {
		return 1
	}
	if ta.kind > tb.kind {
		return -1
	}
	return 1
}
