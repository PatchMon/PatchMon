package docker

import (
	"regexp"
	"strings"

	"github.com/distribution/reference"
)

const noneTag = "<none>"

var (
	fullIDRe   = regexp.MustCompile(`^(?:sha256:)?([a-f0-9]{64})$`)
	prefixIDRe = regexp.MustCompile(`^sha256:([a-f0-9]{12,63})$`)
	shortIDRe  = regexp.MustCompile(`^[a-f0-9]{12,63}$`)
)

type imageRef struct {
	Repository string
	Tag        string
	Digest     string
	ID         string
	// IDOnly is false when ID is a hex string that is also a valid repository name.
	IDOnly bool
}

func (r imageRef) isNamed() bool {
	return r.Repository != ""
}

func parseImageRef(s string) imageRef {
	s = strings.TrimSpace(s)
	if m := fullIDRe.FindStringSubmatch(s); m != nil {
		return imageRef{ID: m[1], IDOnly: true}
	}
	if m := prefixIDRe.FindStringSubmatch(s); m != nil {
		return imageRef{ID: m[1], IDOnly: true}
	}

	named, err := reference.ParseNormalizedNamed(s)
	if err != nil {
		return imageRef{}
	}
	ref := imageRef{Repository: reference.FamiliarName(named)}
	if t, ok := named.(reference.Tagged); ok {
		ref.Tag = t.Tag()
	}
	if d, ok := named.(reference.Digested); ok {
		ref.Digest = d.Digest().String()
	}
	switch {
	case ref.Tag == "" && ref.Digest == "":
		ref.Tag = "latest"
	case ref.Tag == "":
		ref.Tag = noneTag
	}
	if shortIDRe.MatchString(s) {
		ref.ID = s
	}
	return ref
}

func repoTagKey(repository, tag string) string {
	return repository + ":" + tag
}

func repoDigestsFor(repoDigests []string, repository string) []string {
	var out []string
	for _, rd := range repoDigests {
		ref := parseImageRef(rd)
		if ref.Repository == repository && ref.Digest != "" {
			out = append(out, ref.Digest)
		}
	}
	return out
}

func trimSHA(id string) string {
	return strings.TrimPrefix(id, "sha256:")
}
