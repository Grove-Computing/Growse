package style

import (
	"strings"
	"sync"

	"github.com/Grove-Computing/Growse/internal/css"
)

// BrowserUAStylesheetVersion is recorded separately from application CSS so a
// visual baseline can identify exactly which defaults participated in Cascade.
const BrowserUAStylesheetVersion = "growse-browser-ua-v1"

const browserUASource = `
html, body, address, article, aside, footer, header, hgroup, main, nav, section,
div, form, fieldset, figure, figcaption, details, summary { display: block; }
table { display: table; border-collapse: separate; border-spacing: 2px; }
thead, tbody, tfoot { display: table-row-group; }
tr { display: table-row; }
td, th { display: table-cell; padding: 1px; }
th { font-weight: bold; text-align: center; }
head, link, meta, script, style, title, noscript, template, [hidden] { display: none; }
body { margin: 8px; color: #000; font-family: system-ui, sans-serif; font-size: 16px; }
h1 { display: block; font-size: 2em; font-weight: bold; margin-block: .67em; }
h2 { display: block; font-size: 1.5em; font-weight: bold; margin-block: .83em; }
h3 { display: block; font-size: 1.17em; font-weight: bold; margin-block: 1em; }
h4 { display: block; font-size: 1em; font-weight: bold; margin-block: 1.33em; }
h5 { display: block; font-size: .83em; font-weight: bold; margin-block: 1.67em; }
h6 { display: block; font-size: .67em; font-weight: bold; margin-block: 2.33em; }
p { display: block; margin-block: 1em; }
blockquote { display: block; margin-block: 1em; margin-inline: 40px; }
pre { display: block; margin-block: 1em; font-family: monospace; white-space: pre; }
code, kbd, samp, tt { font-family: monospace; }
ul, ol { display: block; margin-block: 1em; padding-inline-start: 40px; }
ol { list-style-type: decimal; }
li { display: block; }
hr { display: block; margin-block: .5em; border-top: 1px solid #808080; }
a { color: #0000ee; text-decoration-line: underline; cursor: pointer; }
b, strong { font-weight: bold; }
small { font-size: .8333em; }
sub, sup { font-size: .8333em; }
img, svg, canvas, video, audio { display: inline; }
iframe { display: inline-block; width: 300px; height: 150px; }
button, input, select, textarea {
  display: inline-block;
  box-sizing: border-box;
  appearance: auto;
  color: #000;
  font-family: system-ui, sans-serif;
  font-size: 13.3333px;
  font-weight: 400;
}
textarea { white-space: pre-wrap; }
button:focus-visible, input:focus-visible, select:focus-visible, textarea:focus-visible, a:focus-visible {
  outline: 2px solid #005fcc;
  outline-offset: 2px;
}
`

var (
	browserUAOnce sync.Once
	browserUA     *css.Stylesheet
)

func browserUAStylesheet() *css.Stylesheet {
	browserUAOnce.Do(func() {
		parsed, err := css.Parse(strings.NewReader(browserUASource))
		if err != nil {
			panic("parse embedded browser UA stylesheet: " + err.Error())
		}
		browserUA = parsed
	})
	return browserUA
}
