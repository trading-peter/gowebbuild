package npmproxy

import (
	"context"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/kataras/golog"
	"github.com/mholt/archiver/v4"
	"github.com/trading-peter/gowebbuild/fsutils"
)

func (p *Proxy) readPackageJson(pkgPath string) (PackageJson, error) {
	pkgFile := filepath.Join(pkgPath, "package.json")

	if !fsutils.IsFile(pkgFile) {
		return PackageJson{}, fmt.Errorf("package.json not found in %s", pkgPath)
	}

	pkgData, err := os.ReadFile(pkgFile)
	if err != nil {
		return PackageJson{}, err
	}

	pkg := PackageJson{}
	err = json.Unmarshal(pkgData, &pkg)
	if err != nil {
		return PackageJson{}, err
	}

	return pkg, nil
}

func (p *Proxy) findDependencyVersionConstraint(projectPkg PackageJson, pkgName string) (*semver.Constraints, error) {
	if verStr, ok := projectPkg.Dependencies[pkgName]; ok {
		return semver.NewConstraint(verStr)
	}

	return nil, fmt.Errorf("package %s not found in project dependencies", pkgName)
}

func (p *Proxy) findPackageSource(override *Override, pkgName string) (*Package, error) {
	pkgNameParts := strings.Split(pkgName, "/")
	pkgPath := filepath.Join(override.PackageRoot, pkgNameParts[len(pkgNameParts)-1])

	// Read the projects package.json and figure out the requested semver version, which probably will be a contraint to assert against (like "^1.2.3").
	projectPkg, err := p.readPackageJson(p.ProjectRoot)
	if err != nil {
		return nil, err
	}

	reqVersion, err := p.findDependencyVersionConstraint(projectPkg, pkgName)

	pkg, err := p.readPackageJson(pkgPath)
	if err != nil {
		return nil, err
	}

	pkgVersion, err := semver.NewVersion(pkg.Version)
	if err != nil {
		return nil, err
	}

	if !reqVersion.Check(pkgVersion) {
		golog.Infof("Version %s in package sources for %s is not meeting the version constrains (%s) of the project. Forwarding request to upstream registry.", pkgVersion, pkgName, reqVersion)
		return nil, nil
	}

	pkgArchive, err := p.createPackage(pkgPath, pkg)
	if err != nil {
		return nil, err
	}

	integrity, shasum, err := p.createHashes(pkgArchive)
	if err != nil {
		return nil, err
	}

	return &Package{
		ID:   pkg.Name,
		Name: pkg.Name,
		DistTags: DistTags{
			Latest: pkg.Version,
		},
		Versions: map[string]Version{
			pkg.Version: {
				ID:           pkg.Name,
				Name:         pkg.Name,
				Version:      pkg.Version,
				Dependencies: pkg.Dependencies,
				Dist: Dist{
					Integrity: integrity,
					Shasum:    shasum,
					Tarball:   fmt.Sprintf("%s/files/%s", p.internalProxyUrl, filepath.Base(pkgArchive)),
				},
			},
		},
	}, nil
}

func (p *Proxy) createPackage(pkgPath string, pkg PackageJson) (string, error) {
	err := os.MkdirAll(p.pkgCachePath, 0755)
	if err != nil {
		return "", err
	}

	pkgArchive := filepath.Join(p.pkgCachePath, sanitizePkgName(pkg.Name, pkg.Version)+".tar")

	files, err := archiver.FilesFromDisk(nil, map[string]string{
		pkgPath: ".",
	})

	if err != nil {
		return "", err
	}

	filesFiltered := []archiver.File{}

	filterRegex := regexp.MustCompile(`^node_modules|.git`)

	for _, file := range files {
		if filterRegex.MatchString(file.NameInArchive) {
			continue
		}
		filesFiltered = append(filesFiltered, file)
	}

	out, err := os.Create(pkgArchive)
	if err != nil {
		return "", err
	}
	defer out.Close()

	format := archiver.CompressedArchive{
		Archival: archiver.Tar{},
	}

	err = format.Archive(context.Background(), out, filesFiltered)
	if err != nil {
		return "", err
	}

	return pkgArchive, nil
}

func (p *Proxy) createHashes(pkgArchive string) (string, string, error) {
	// Open the file
	file, err := os.Open(pkgArchive)
	if err != nil {
		return "", "", err
	}
	defer file.Close()

	// Create a new SHA256 hash
	hash := sha512.New()

	// Copy the file data into the hash
	if _, err := io.Copy(hash, file); err != nil {
		return "", "", err
	}

	// Get the hash sum
	hashSum := hash.Sum(nil)

	// Generate the integrity string (SHA-256 base64-encoded)
	integrity := "sha512-" + base64.StdEncoding.EncodeToString(hashSum)

	// Generate the shasum (hexadecimal representation)
	shasum := fmt.Sprintf("%x", hashSum)

	return integrity, shasum, nil
}

// Replace all characters of the pages name that are not allowed in a URL with a hyphen.
func sanitizePkgName(pkgName string, version string) string {
	pkgName = strings.ReplaceAll(pkgName, "@", "")
	pkgName = strings.ReplaceAll(pkgName, "/", "_")
	version = strings.ReplaceAll(version, ".", "_")
	return fmt.Sprintf("%s_%s", pkgName, version)
}
