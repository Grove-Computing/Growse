package layout

import (
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/style"
)

func TestFloatClearAndReplacedIntrinsicRatioParticipateInBlockFlow(t *testing.T) {
	document := dom.NewDocument()
	container := document.CreateElement("div", map[string]string{"class": "container"})
	image := document.CreateElement("img", map[string]string{"class": "lead", "src": "wide.png"})
	paragraph := document.CreateElement("p", nil)
	clear := document.CreateElement("div", map[string]string{"class": "clear"})
	appendNodes(t, document,
		[2]*dom.Node{document.Root, container}, [2]*dom.Node{container, image},
		[2]*dom.Node{container, paragraph}, [2]*dom.Node{paragraph, document.CreateText("Words wrap beside a floated replaced element before continuing below it.")},
		[2]*dom.Node{container, clear}, [2]*dom.Node{clear, document.CreateText("cleared")},
	)
	stylesheet, err := css.Parse(strings.NewReader(`
.container { display:block; width:300px; background:#eee }
.lead { float:left; width:120px; margin-right:10px }
p { display:block; margin:0 }
.clear { display:block; clear:both; height:20px }
`))
	if err != nil {
		t.Fatal(err)
	}
	computed := style.Compute(document, stylesheet)
	imageStyle, _ := computed.For(image)
	clearStyle, _ := computed.For(clear)
	if imageStyle.Float != style.FloatLeft || clearStyle.Clear != style.ClearBoth {
		t.Fatalf("float/clear styles = %v/%v", imageStyle.Float, clearStyle.Clear)
	}
	tree := BuildWithScrollAndImages(document, computed, map[dom.NodeID]ImageResource{
		image.ID: {URL: "wide.png", IntrinsicWidth: 200, IntrinsicHeight: 100, Loaded: true},
	}, 500, 400, 0, 0)
	imageRect := tree.Bounds[image.ID]
	paragraphRect := tree.Bounds[paragraph.ID]
	clearRect := tree.Bounds[clear.ID]
	if imageRect.Width != 120 || imageRect.Height != 60 {
		t.Fatalf("floated replaced ratio = %#v, want 120x60", imageRect)
	}
	var wrapped Box
	for _, box := range tree.Boxes {
		if box.NodeID == paragraph.ID && box.Y < imageRect.Y+imageRect.Height {
			wrapped = box
			break
		}
	}
	if wrapped.Width == 0 || wrapped.X < imageRect.X+imageRect.Width+10 || wrapped.Width > 170 {
		t.Fatalf("text did not wrap around float: image=%#v text=%#v paragraph=%#v", imageRect, wrapped, paragraphRect)
	}
	if clearRect.Y < imageRect.Y+imageRect.Height {
		t.Fatalf("clear box y=%v, want at or below float bottom %v", clearRect.Y, imageRect.Y+imageRect.Height)
	}
	containerRect := tree.Bounds[container.ID]
	if containerRect.Height < clearRect.Y+clearRect.Height-containerRect.Y {
		t.Fatalf("parent height %#v does not contain cleared child %#v", containerRect, clearRect)
	}
}
