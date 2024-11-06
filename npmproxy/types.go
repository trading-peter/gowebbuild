package npmproxy

type PackageJson struct {
	Name         string            `json:"name"`
	Version      string            `json:"version"`
	Dependencies map[string]string `json:"dependencies"`
}

type Package struct {
	ID          string             `json:"_id"`
	Name        string             `json:"name"`
	Description string             `json:"description"`
	DistTags    DistTags           `json:"dist-tags"`
	Versions    map[string]Version `json:"versions"`
	Readme      string             `json:"readme"`
	Repository  Repository         `json:"repository"`
	Author      Author             `json:"author"`
	License     string             `json:"license"`
}

type DistTags struct {
	Latest string `json:"latest"`
}

type Author struct {
	Name string `json:"name"`
}

type Repository struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

type Dist struct {
	Integrity string `json:"integrity"`
	Shasum    string `json:"shasum"`
	Tarball   string `json:"tarball"`
}

type Version struct {
	ID           string            `json:"_id"`
	Name         string            `json:"name"`
	Version      string            `json:"version"`
	Description  string            `json:"description"`
	Author       Author            `json:"author"`
	License      string            `json:"license"`
	Repository   Repository        `json:"repository"`
	Dependencies map[string]string `json:"dependencies"`
	Readme       string            `json:"readme"`
	Dist         Dist              `json:"dist"`
}
