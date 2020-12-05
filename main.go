package main

import (
	"io/ioutil"
	"path/filepath"
	"strings"

	"github.com/operator-framework/operator-registry/pkg/registry"
	errors2 "github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

var rootCmd = &cobra.Command{
	Short: "olm-configmap-gen",
	PreRunE: func(cmd *cobra.Command, args []string) error {
		if debug, _ := cmd.Flags().GetBool("debug"); debug {
			logrus.SetLevel(logrus.DebugLevel)
		}
		return nil
	},

	RunE: runCmdFunc,
}

func init() {
	rootCmd.Flags().Bool("debug", true, "enable debug logging")
	rootCmd.Flags().String("in", "", "input dir name")
	rootCmd.Flags().String("out", "", "output file name")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		logrus.Error(err.Error())
	}
}

func runCmdFunc(cmd *cobra.Command, args []string) error {
	in, err := cmd.Flags().GetString("in")
	if err != nil {
		return err
	}

	out, err := cmd.Flags().GetString("out")
	if err != nil {
		return err
	}

	return genConfigmap(in, out)
}

func genConfigmap(in, out string) error {
	fis, err := ioutil.ReadDir(in)
	if err != nil {
		return errors2.WithStack(err)
	}

	var crds []unstructured.Unstructured
	var csvs []unstructured.Unstructured
	var others []unstructured.Unstructured

	for _, fi := range fis {
		if fi.IsDir() {
			continue
		}

		ym, err := ioutil.ReadFile(filepath.Join(in, fi.Name()))
		if err != nil {
			return errors2.WithStack(err)
		}
		js, err := yaml.YAMLToJSON(ym)
		if err != nil {
			return errors2.WithStack(err)
		}

		un := unstructured.Unstructured{}
		err = un.UnmarshalJSON(js)
		if err != nil {
			return errors2.WithStack(err)
		}

		switch un.GetKind() {
		case "CustomResourceDefinition":
			crds = append(crds, un)
		case "ClusterServiceVersion":
			csvs = append(csvs, un)
		default:
			others = append(others, un)
		}

	}

	csvName := csvs[0].GetName()
	pkgName := csvName[:strings.Index(csvName, ".")]
	defChannel := "stable"

	pkgs := []registry.PackageManifest{
		{
			PackageName:        pkgName,
			DefaultChannelName: defChannel,
			Channels: []registry.PackageChannel{
				{
					Name:           defChannel,
					CurrentCSVName: csvName,
				},
			},
		},
	}
	pkgY, err := yaml.Marshal(pkgs)
	if err != nil {
		return errors2.WithStack(err)
	}

	cm := corev1.ConfigMap{}
	cm.Kind = "ConfigMap"
	cm.APIVersion = "v1"
	cm.Name = "catalogsrouce-" + pkgName
	cm.Data = map[string]string{
		"customResourceDefinitions": mustYamlArrToString(crds),
		"clusterServiceVersions":    mustYamlArrToString(csvs),
		"packages":                  string(pkgY),
	}

	if len(others) > 0 {
		cm.Data["customResourceYaml"] = mustYamlArrToString(others)
	}

	cmY, err := yaml.Marshal(cm)
	if err != nil {
		return errors2.WithStack(err)
	}

	err = ioutil.WriteFile(out, cmY, 0664)
	if err != nil {
		return errors2.WithStack(err)
	}
	return nil
}

func mustYamlArrToString(in []unstructured.Unstructured) string {
	if len(in) == 0 {
		return ""
	}

	r, err := yaml.Marshal(in)
	if err != nil {
		panic(err)
	}
	return string(r)
}
