// llbdump converts a Dockerfile to LLB the way the BuildKit Dockerfile frontend does
// and prints one line per vertex: "<digest> <name>". Two variants (build args,
// target) that print the same digest for a vertex share its cache key.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/containerd/platforms"
	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/frontend/dockerfile/dockerfile2llb"
	"github.com/moby/buildkit/frontend/dockerui"
	"github.com/moby/buildkit/solver/pb"
	digest "github.com/opencontainers/go-digest"
	"google.golang.org/protobuf/encoding/prototext"
)

type multi []string

func (m *multi) String() string     { return strings.Join(*m, ",") }
func (m *multi) Set(v string) error { *m = append(*m, v); return nil }

func main() {
	file := flag.String("f", "Dockerfile", "Dockerfile")
	target := flag.String("target", "", "target stage")
	plat := flag.String("platform", "linux/amd64", "target platform")
	var args multi
	flag.Var(&args, "arg", "build arg k=v")
	flag.Parse()

	dt, err := os.ReadFile(*file)
	if err != nil {
		panic(err)
	}
	ba := map[string]string{}
	for _, a := range args {
		k, v, _ := strings.Cut(a, "=")
		ba[k] = v
	}
	p, err := platforms.Parse(*plat)
	if err != nil {
		panic(err)
	}
	ctx := context.Background()
	// A fixed main context: without one the converter makes a local source with
	// a random unique ID, so every run would print different digests.
	mainCtx := llb.Local("context", llb.SharedKeyHint("context"), llb.LocalUniqueID("fixed"), llb.WithCustomName("[internal] load build context"))
	res, err := dockerfile2llb.Dockerfile2LLB(ctx, dt, dockerfile2llb.ConvertOpt{
		Config: dockerui.Config{
			BuildArgs: ba,
			Target:    *target,
		},
		TargetPlatform: &p,
		MainContext:    &mainCtx,
	})
	if err != nil {
		panic(err)
	}
	def, err := res.State.Marshal(ctx, llb.Platform(p))
	if err != nil {
		panic(err)
	}
	var lines []string
	for _, b := range def.Def {
		dgst := digest.FromBytes(b)
		var op pb.Op
		if err := op.UnmarshalVT(b); err != nil {
			panic(err)
		}
		name := ""
		if md, ok := def.Metadata[dgst]; ok && md.Description != nil {
			name = md.Description["llb.customname"]
		}
		if name == "" {
			name = "(unnamed " + fmt.Sprintf("%T", op.Op) + ")"
		}
		if dn := os.Getenv("DUMP"); dn != "" && strings.Contains(name, dn) {
			fmt.Fprintf(os.Stderr, "%s\n%s\n", name, prototext.Format(&op))
		}
		lines = append(lines, fmt.Sprintf("%s %s", dgst, name))
	}
	sort.Slice(lines, func(i, j int) bool { return lines[i][72:] < lines[j][72:] })
	for _, l := range lines {
		fmt.Println(l)
	}
}
