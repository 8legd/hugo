// Copyright © 2014 Steve Francia <spf@spf13.com>.
//
// Licensed under the Simple Public License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
// http://opensource.org/licenses/Simple-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package create

import (
	"bytes"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cast"
	"github.com/spf13/hugo/helpers"
	"github.com/spf13/hugo/hugofs"
	"github.com/spf13/hugo/hugolib"
	"github.com/spf13/hugo/parser"
	jww "github.com/spf13/jwalterweatherman"
	"github.com/spf13/viper"
)

func NewContent(kind, name string) (err error) {
	jww.INFO.Println("attempting to create ", name, "of", kind)

	location := FindArchetype(kind)

	var by []byte

	if location != "" {
		by, err = ioutil.ReadFile(location)
		if err != nil {
			jww.ERROR.Println(err)
		}
	}
	if location == "" || err != nil {
		by = []byte("+++\n title = \"title\"\n draft = true \n+++\n")
	}

	psr, err := parser.ReadFrom(bytes.NewReader(by))
	if err != nil {
		return err
	}
	metadata, err := psr.Metadata()
	if err != nil {
		return err
	}
	newmetadata, err := cast.ToStringMapE(metadata)
	if err != nil {
		jww.ERROR.Println("Error processing archetype file:", location)
		return err
	}

	for k := range newmetadata {
		switch strings.ToLower(k) {
		case "date":
			newmetadata[k] = time.Now()
		case "title":
			newmetadata[k] = helpers.MakeTitle(helpers.Filename(name))
		}
	}

	caseimatch := func(m map[string]interface{}, key string) bool {
		for k := range m {
			if strings.ToLower(k) == strings.ToLower(key) {
				return true
			}
		}
		return false
	}

	if newmetadata == nil {
		newmetadata = make(map[string]interface{})
	}

	if !caseimatch(newmetadata, "date") {
		newmetadata["date"] = time.Now()
	}

	if !caseimatch(newmetadata, "title") {
		newmetadata["title"] = helpers.MakeTitle(helpers.Filename(name))
	}

	page, err := hugolib.NewPage(name)
	if err != nil {
		return err
	}

	if x := parser.FormatSanitize(viper.GetString("MetaDataFormat")); x == "json" || x == "yaml" || x == "toml" {
		newmetadata["date"] = time.Now().Format(time.RFC3339)
	}

	// tweaks
	// TODO think about how these kind of tweaks could be added in a more flexible way to hugo and create feature request / PR
	// 	maybe add a `special` section/table to the archetype front matter e.g. [mappings] which would contain entries
	// 	like menu.main.name = "title" and  menu.main.weight = "$count" (look at similar functionality in hugo elsewhere for naming etc)

	// Make `title` Title case
	if str, ok := newmetadata["title"].(string); ok {
		newmetadata["title"] = strings.Title(str)
	}

	// Support generation of `name` and `weight` on any menus
	// `name` defaults to `title`
	// `weight` defaults to a count of existing content + 1
	if menu, ok := newmetadata["menu"].(map[string]interface{}); ok {
		for j := range menu {
			if menumeta, ok := menu[j].(map[string]interface{}); ok {
				for l := range menumeta {
					switch strings.ToLower(l) {
					case "name":
						menumeta[l] = newmetadata["title"]
					case "weight":
						files, err := ioutil.ReadDir(filepath.Dir(filepath.Join(viper.GetString("contentDir"), name)))
						if err != nil {
							menumeta[l] = 0
						} else {
							menumeta[l] = len(files) + 1
						}
					}
				}
			}
		}
	}
	// /tweaks

	//page.Dir = viper.GetString("sourceDir")
	page.SetSourceMetaData(newmetadata, parser.FormatToLeadRune(viper.GetString("MetaDataFormat")))
	page.SetSourceContent(psr.Content())
	if err = page.SafeSaveSourceAs(filepath.Join(viper.GetString("contentDir"), name)); err != nil {
		return
	}
	jww.FEEDBACK.Println(helpers.AbsPathify(filepath.Join(viper.GetString("contentDir"), name)), "created")

	editor := viper.GetString("NewContentEditor")

	if editor != "" {
		jww.FEEDBACK.Printf("Editing %s in %s.\n", name, editor)

		cmd := exec.Command(editor, path.Join(viper.GetString("contentDir"), name))
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err = cmd.Run(); err != nil {
			return
		}
	}

	return nil
}

func FindArchetype(kind string) (outpath string) {
	search := []string{helpers.AbsPathify(viper.GetString("archetypeDir"))}

	if viper.GetString("theme") != "" {
		themeDir := filepath.Join(helpers.AbsPathify("themes/"+viper.GetString("theme")), "/archetypes/")
		if _, err := os.Stat(themeDir); os.IsNotExist(err) {
			jww.ERROR.Println("Unable to find archetypes directory for theme :", viper.GetString("theme"), "in", themeDir)
		} else {
			search = append(search, themeDir)
		}
	}

	for _, x := range search {
		// If the new content isn't in a subdirectory, kind == "".
		// Therefore it should be excluded otherwise `is a directory`
		// error will occur. github.com/spf13/hugo/issues/411
		var pathsToCheck []string

		if kind == "" {
			pathsToCheck = []string{"default.md", "default"}
		} else {
			pathsToCheck = []string{kind + ".md", kind, "default.md", "default"}
		}
		for _, p := range pathsToCheck {
			curpath := filepath.Join(x, p)
			jww.DEBUG.Println("checking", curpath, "for archetypes")
			if exists, _ := helpers.Exists(curpath, hugofs.SourceFs); exists {
				jww.INFO.Println("curpath: " + curpath)
				return curpath
			}
		}
	}

	return ""
}
