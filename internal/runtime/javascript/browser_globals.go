package javascript

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/dop251/goja"
)

const (
	maxEncodedTextBytes = 1 << 20
	maxRandomBytes      = 65_536
	maxURLBytes         = 1 << 20
)

func (runtime *Runtime) installBrowserGlobals(vm *goja.Runtime) error {
	origin := time.Now()
	resolveURL := func(call goja.FunctionCall) goja.Value {
		base := ""
		if value := call.Argument(1); value != nil && !goja.IsUndefined(value) {
			base = value.String()
		}
		result, err := browserURL(call.Argument(0).String(), base)
		if err != nil {
			panic(vm.NewTypeError("invalid URL"))
		}
		return vm.ToValue(result)
	}
	mutateURL := func(call goja.FunctionCall) goja.Value {
		result, err := mutateBrowserURL(call.Argument(0).String(), call.Argument(1).String(), call.Argument(2).String())
		if err != nil {
			panic(vm.NewTypeError("invalid URL"))
		}
		return vm.ToValue(result)
	}
	encodeText := func(call goja.FunctionCall) goja.Value {
		encoded := []byte(call.Argument(0).String())
		if len(encoded) > maxEncodedTextBytes {
			panic(vm.NewTypeError("encoded text exceeds 1 MiB"))
		}
		return vm.ToValue(vm.NewArrayBuffer(encoded))
	}
	decodeText := func(call goja.FunctionCall) goja.Value {
		var encoded []byte
		value := call.Argument(0)
		if value != nil && !goja.IsUndefined(value) {
			if err := vm.ExportTo(value, &encoded); err != nil {
				panic(vm.NewTypeError("TextDecoder input must be an ArrayBuffer or typed array"))
			}
		}
		if len(encoded) > maxEncodedTextBytes {
			panic(vm.NewTypeError("encoded text exceeds 1 MiB"))
		}
		fatal := jsBoolean(call.Argument(1))
		if fatal && !utf8.Valid(encoded) {
			panic(vm.NewTypeError("encoded text is not valid UTF-8"))
		}
		return vm.ToValue(strings.ToValidUTF8(string(encoded), "\uFFFD"))
	}
	decodeBase64 := func(call goja.FunctionCall) goja.Value {
		input := strings.Map(func(value rune) rune {
			if value == ' ' || value == '\t' || value == '\n' || value == '\r' || value == '\f' {
				return -1
			}
			return value
		}, call.Argument(0).String())
		if len(input) > maxEncodedTextBytes {
			panic(vm.NewTypeError("base64 input exceeds 1 MiB"))
		}
		decoded, err := base64.StdEncoding.DecodeString(input)
		if err != nil {
			decoded, err = base64.RawStdEncoding.DecodeString(input)
		}
		if err != nil {
			panic(vm.NewTypeError("invalid base64 input"))
		}
		var output strings.Builder
		output.Grow(len(decoded))
		for _, value := range decoded {
			output.WriteRune(rune(value))
		}
		return vm.ToValue(output.String())
	}
	encodeBase64 := func(call goja.FunctionCall) goja.Value {
		input := call.Argument(0).String()
		if len(input) > maxEncodedTextBytes {
			panic(vm.NewTypeError("base64 input exceeds 1 MiB"))
		}
		bytes := make([]byte, 0, len(input))
		for _, value := range input {
			if value > 255 {
				panic(vm.NewTypeError("btoa input contains a non-Latin1 character"))
			}
			bytes = append(bytes, byte(value))
		}
		return vm.ToValue(base64.StdEncoding.EncodeToString(bytes))
	}
	fillRandom := func(call goja.FunctionCall) goja.Value {
		var target []byte
		if err := vm.ExportTo(call.Argument(0), &target); err != nil {
			panic(vm.NewTypeError("getRandomValues requires an integer typed array"))
		}
		if len(target) > maxRandomBytes {
			panic(vm.NewTypeError("random buffer exceeds 65536 bytes"))
		}
		if _, err := rand.Read(target); err != nil {
			panic(vm.NewGoError(fmt.Errorf("generate random values: %w", err)))
		}
		return call.Argument(0)
	}

	hosts := map[string]interface{}{
		"__growseResolveURL":   resolveURL,
		"__growseMutateURL":    mutateURL,
		"__growseEncodeText":   encodeText,
		"__growseDecodeText":   decodeText,
		"__growseDecodeBase64": decodeBase64,
		"__growseEncodeBase64": encodeBase64,
		"__growseFillRandom":   fillRandom,
	}
	for name, host := range hosts {
		if err := vm.Set(name, host); err != nil {
			return err
		}
	}
	if _, err := vm.RunString(browserGlobalsSource); err != nil {
		return fmt.Errorf("install browser globals: %w", err)
	}
	performance := vm.NewObject()
	if err := performance.Set("now", func(goja.FunctionCall) goja.Value {
		return vm.ToValue(float64(time.Since(origin)) / float64(time.Millisecond))
	}); err != nil {
		return err
	}
	if err := performance.DefineDataProperty("timeOrigin", vm.ToValue(float64(origin.UnixNano())/float64(time.Millisecond)), goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE); err != nil {
		return err
	}
	return vm.Set("performance", performance)
}

func browserURL(raw, base string) (map[string]string, error) {
	if len(raw) > maxURLBytes || len(base) > maxURLBytes {
		return nil, errorsURLTooLarge
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if !parsed.IsAbs() {
		if base == "" {
			return nil, fmt.Errorf("relative URL without base")
		}
		baseValue, err := url.Parse(base)
		if err != nil || !baseValue.IsAbs() {
			return nil, fmt.Errorf("invalid base URL")
		}
		parsed = baseValue.ResolveReference(parsed)
	}
	if parsed.Scheme == "" {
		return nil, fmt.Errorf("URL has no scheme")
	}
	return browserURLFields(parsed), nil
}

var errorsURLTooLarge = fmt.Errorf("URL exceeds 1 MiB")

func mutateBrowserURL(href, field, value string) (map[string]string, error) {
	if len(href) > maxURLBytes || len(value) > maxURLBytes {
		return nil, errorsURLTooLarge
	}
	parsed, err := url.Parse(href)
	if err != nil || !parsed.IsAbs() {
		return nil, fmt.Errorf("invalid URL")
	}
	switch field {
	case "protocol":
		parsed.Scheme = strings.TrimSuffix(value, ":")
	case "username", "password":
		username := parsed.User.Username()
		password, hasPassword := parsed.User.Password()
		if field == "username" {
			username = value
		} else {
			password, hasPassword = value, true
		}
		if hasPassword {
			parsed.User = url.UserPassword(username, password)
		} else {
			parsed.User = url.User(username)
		}
	case "host":
		parsed.Host = value
	case "hostname":
		port := parsed.Port()
		parsed.Host = value
		if port != "" {
			parsed.Host += ":" + port
		}
	case "port":
		parsed.Host = parsed.Hostname()
		if value != "" {
			parsed.Host += ":" + value
		}
	case "pathname":
		parsed.Path, parsed.RawPath = value, ""
	case "search":
		parsed.RawQuery = strings.TrimPrefix(value, "?")
	case "hash":
		parsed.Fragment, parsed.RawFragment = strings.TrimPrefix(value, "#"), ""
	default:
		return nil, fmt.Errorf("unsupported URL field")
	}
	return browserURLFields(parsed), nil
}

func browserURLFields(parsed *url.URL) map[string]string {
	password, _ := parsed.User.Password()
	pathname := parsed.EscapedPath()
	if pathname == "" && parsed.Host != "" {
		pathname = "/"
	}
	origin := "null"
	if parsed.Scheme == "http" || parsed.Scheme == "https" {
		origin = parsed.Scheme + "://" + parsed.Host
	}
	search, hash := "", ""
	if parsed.RawQuery != "" || parsed.ForceQuery {
		search = "?" + parsed.RawQuery
	}
	if parsed.Fragment != "" {
		hash = "#" + parsed.EscapedFragment()
	}
	return map[string]string{
		"href": parsed.String(), "origin": origin, "protocol": parsed.Scheme + ":",
		"username": parsed.User.Username(), "password": password, "host": parsed.Host,
		"hostname": parsed.Hostname(), "port": parsed.Port(), "pathname": pathname,
		"search": search, "hash": hash,
	}
}

const browserGlobalsSource = `
(function (global, resolveURL, mutateURL, encodeText, decodeText, decodeBase64, encodeBase64, fillRandom) {
  "use strict";
  const MAX_PARAMS = 8192;
  function formDecode(value) { return decodeURIComponent(String(value).replace(/\+/g, " ")); }
  function formEncode(value) {
    return encodeURIComponent(String(value)).replace(/%20/g, "+").replace(/[!'()~]/g, function (c) {
      return "%" + c.charCodeAt(0).toString(16).toUpperCase();
    });
  }
  function parseParams(value) {
    if (value === "") return [];
    return String(value).replace(/^\?/, "").split("&").map(function (part) {
      const index = part.indexOf("=");
      return index < 0 ? [formDecode(part), ""] : [formDecode(part.slice(0, index)), formDecode(part.slice(index + 1))];
    });
  }
  function URLSearchParams(init, onChange) {
    if (!(this instanceof URLSearchParams)) throw new TypeError("URLSearchParams constructor requires new");
    this._entries = [];
    this._onChange = typeof onChange === "function" ? onChange : null;
    if (init == null) return;
    if (init instanceof URLSearchParams) this._entries = init._entries.map(function (entry) { return entry.slice(); });
    else if (typeof init === "string") this._entries = parseParams(init);
    else if (typeof init[Symbol.iterator] === "function") {
      for (const entry of init) {
        if (!entry || typeof entry[Symbol.iterator] !== "function") throw new TypeError("URLSearchParams entry must be a pair");
        const pair = Array.from(entry);
        if (pair.length !== 2) throw new TypeError("URLSearchParams entry must contain two values");
        this._entries.push([String(pair[0]), String(pair[1])]);
      }
    } else {
      for (const key of Object.keys(Object(init))) this._entries.push([key, String(init[key])]);
    }
    this._validate();
  }
  URLSearchParams.prototype._validate = function () {
    if (this.toString().length > MAX_PARAMS) throw new RangeError("URLSearchParams exceeds size limit");
  };
  URLSearchParams.prototype._changed = function () { this._validate(); if (this._onChange) this._onChange(this.toString()); };
  URLSearchParams.prototype._replace = function (value) { this._entries = parseParams(value); this._validate(); };
  URLSearchParams.prototype.append = function (name, value) { this._entries.push([String(name), String(value)]); this._changed(); };
  URLSearchParams.prototype.delete = function (name, value) {
    name = String(name); const hasValue = arguments.length > 1; value = String(value);
    this._entries = this._entries.filter(function (entry) { return entry[0] !== name || (hasValue && entry[1] !== value); }); this._changed();
  };
  URLSearchParams.prototype.get = function (name) { name = String(name); const found = this._entries.find(function (e) { return e[0] === name; }); return found ? found[1] : null; };
  URLSearchParams.prototype.getAll = function (name) { name = String(name); return this._entries.filter(function (e) { return e[0] === name; }).map(function (e) { return e[1]; }); };
  URLSearchParams.prototype.has = function (name, value) { name = String(name); const hasValue = arguments.length > 1; value = String(value); return this._entries.some(function (e) { return e[0] === name && (!hasValue || e[1] === value); }); };
  URLSearchParams.prototype.set = function (name, value) {
    name = String(name); value = String(value); let found = false; const next = [];
    for (const entry of this._entries) { if (entry[0] !== name) next.push(entry); else if (!found) { next.push([name, value]); found = true; } }
    if (!found) next.push([name, value]); this._entries = next; this._changed();
  };
  URLSearchParams.prototype.sort = function () { this._entries = this._entries.map(function(e, i) { return [e, i]; }).sort(function(a,b) { return a[0][0] < b[0][0] ? -1 : a[0][0] > b[0][0] ? 1 : a[1]-b[1]; }).map(function(e) { return e[0]; }); this._changed(); };
  URLSearchParams.prototype.toString = function () { return this._entries.map(function (e) { return formEncode(e[0]) + "=" + formEncode(e[1]); }).join("&"); };
  URLSearchParams.prototype.entries = function () { return this._entries.map(function(e) { return e.slice(); })[Symbol.iterator](); };
  URLSearchParams.prototype.keys = function () { return this._entries.map(function(e) { return e[0]; })[Symbol.iterator](); };
  URLSearchParams.prototype.values = function () { return this._entries.map(function(e) { return e[1]; })[Symbol.iterator](); };
  URLSearchParams.prototype.forEach = function (callback, thisArg) { for (const entry of this._entries.slice()) callback.call(thisArg, entry[1], entry[0], this); };
  URLSearchParams.prototype[Symbol.iterator] = URLSearchParams.prototype.entries;
  Object.defineProperty(URLSearchParams.prototype, "size", { get: function () { return this._entries.length; } });

  function URL(input, base) {
    if (!(this instanceof URL)) throw new TypeError("URL constructor requires new");
    let state = resolveURL(String(input), base === undefined ? undefined : String(base));
    const self = this;
    const params = new URLSearchParams(state.search, function (query) { state = mutateURL(state.href, "search", query); });
    function update(next) { state = next; params._replace(state.search); }
    Object.defineProperties(this, {
      href: { enumerable: true, get: function () { return state.href; }, set: function(v) { update(resolveURL(String(v), undefined)); } },
      origin: { enumerable: true, get: function () { return state.origin; } },
      searchParams: { enumerable: true, get: function () { return params; } }
    });
    for (const field of ["protocol", "username", "password", "host", "hostname", "port", "pathname", "search", "hash"]) {
      Object.defineProperty(self, field, { enumerable: true, get: function () { return state[field]; }, set: function(v) { update(mutateURL(state.href, field, String(v))); } });
    }
  }
  URL.prototype.toString = function () { return this.href; };
  URL.prototype.toJSON = function () { return this.href; };
  URL.canParse = function (input, base) { try { resolveURL(String(input), base === undefined ? undefined : String(base)); return true; } catch (_) { return false; } };

  function TextEncoder() { if (!(this instanceof TextEncoder)) throw new TypeError("TextEncoder constructor requires new"); }
  Object.defineProperty(TextEncoder.prototype, "encoding", { get: function () { return "utf-8"; } });
  TextEncoder.prototype.encode = function (input) { return new Uint8Array(encodeText(input === undefined ? "" : String(input))); };
  TextEncoder.prototype.encodeInto = function (input, destination) {
    input = String(input); if (!(destination instanceof Uint8Array)) throw new TypeError("destination must be Uint8Array");
    const bytes = this.encode(input); const written = Math.min(bytes.length, destination.length); destination.set(bytes.subarray(0, written));
    let read = 0, used = 0; for (const character of input) { const size = this.encode(character).length; if (used + size > written) break; used += size; read += character.length; }
    return { read: read, written: used };
  };
  function TextDecoder(label, options) {
    if (!(this instanceof TextDecoder)) throw new TypeError("TextDecoder constructor requires new");
    label = label === undefined ? "utf-8" : String(label).trim().toLowerCase();
    if (["utf-8", "utf8", "unicode-1-1-utf-8"].indexOf(label) < 0) throw new RangeError("unsupported encoding");
    options = options == null ? {} : Object(options); this._fatal = Boolean(options.fatal); this._ignoreBOM = Boolean(options.ignoreBOM);
  }
  Object.defineProperties(TextDecoder.prototype, { encoding: { get: function () { return "utf-8"; } }, fatal: { get: function () { return this._fatal; } }, ignoreBOM: { get: function () { return this._ignoreBOM; } } });
  TextDecoder.prototype.decode = function (input, options) { if (options && options.stream) throw new TypeError("streaming decode is unsupported"); let output = decodeText(input, this._fatal); if (!this._ignoreBOM && output.charCodeAt(0) === 0xFEFF) output = output.slice(1); return output; };

  global.URL = URL; global.URLSearchParams = URLSearchParams; global.TextEncoder = TextEncoder; global.TextDecoder = TextDecoder;
  global.atob = function (value) { return decodeBase64(String(value)); };
  global.btoa = function (value) { return encodeBase64(String(value)); };
  const crypto = {};
  crypto.getRandomValues = function (target) {
    const tag = Object.prototype.toString.call(target);
    if (["[object Int8Array]", "[object Uint8Array]", "[object Uint8ClampedArray]", "[object Int16Array]", "[object Uint16Array]", "[object Int32Array]", "[object Uint32Array]", "[object BigInt64Array]", "[object BigUint64Array]"].indexOf(tag) < 0) throw new TypeError("getRandomValues requires an integer typed array");
    return fillRandom(target);
  };
  global.crypto = crypto;
})(globalThis, __growseResolveURL, __growseMutateURL, __growseEncodeText, __growseDecodeText, __growseDecodeBase64, __growseEncodeBase64, __growseFillRandom);
delete globalThis.__growseResolveURL; delete globalThis.__growseMutateURL; delete globalThis.__growseEncodeText; delete globalThis.__growseDecodeText;
delete globalThis.__growseDecodeBase64; delete globalThis.__growseEncodeBase64; delete globalThis.__growseFillRandom;
`
