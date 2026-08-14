#!/usr/bin/env bash

set -euo pipefail

color_mode="${GROWSE_COLOR:-auto}"
color_enabled=false
case "$color_mode" in
  always)
    color_enabled=true
    ;;
  never)
    ;;
  auto)
    if [[ -t 1 && "${TERM:-}" != "dumb" && -z "${NO_COLOR+x}" ]]; then
      color_enabled=true
    fi
    ;;
  *)
    printf '無効なGROWSE_COLORです: %s（auto、always、neverから選択してください）\n' "$color_mode" >&2
    exit 1
    ;;
esac

if [[ "$color_enabled" == true ]]; then
  color_blue=$'\033[38;2;66;99;235m'
  color_green=$'\033[38;2;47;158;68m'
  color_yellow=$'\033[38;2;240;140;0m'
  color_red=$'\033[38;2;224;49;49m'
  color_muted=$'\033[2m'
  color_bold=$'\033[1m'
  color_reset=$'\033[0m'
else
  color_blue=
  color_green=
  color_yellow=
  color_red=
  color_muted=
  color_bold=
  color_reset=
fi

print_banner() {
  printf '\n%b%b  Growse Installer%b\n' "$color_bold" "$color_blue" "$color_reset"
  printf '%b  WebGoで描く、小さなブラウザ%b\n\n' "$color_muted" "$color_reset"
}

step() {
  local current=$1
  local total=$2
  shift 2
  printf '%b[%s/%s]%b %s\n' "$color_blue" "$current" "$total" "$color_reset" "$*"
}

info() {
  printf '      %b%s%b\n' "$color_muted" "$*" "$color_reset"
}

success() {
  printf '  %b✓%b %s\n' "$color_green" "$color_reset" "$*"
}

warn() {
  printf '  %b!%b %s\n' "$color_yellow" "$color_reset" "$*" >&2
}

die() {
  printf '  %b✗%b %s\n' "$color_red" "$color_reset" "$*" >&2
  exit 1
}

repository="${GROWSE_REPOSITORY:-Grove-Computing/Growse}"
version="${GROWSE_VERSION:-latest}"
install_dir="${GROWSE_INSTALL_DIR:-${HOME}/.local/bin}"
data_home="${GROWSE_DATA_HOME:-${XDG_DATA_HOME:-${HOME}/.local/share}}"
applications_dir="${GROWSE_APPLICATIONS_DIR:-${HOME}/Applications}"
api_base_url="${GROWSE_API_BASE_URL:-https://api.github.com/repos/${repository}}"
release_base_url="${GROWSE_RELEASE_BASE_URL:-https://github.com/${repository}/releases/download}"
app_id="io.github.grovecomputing.Growse"

download() {
  local url=$1
  local output=$2

  if command -v curl >/dev/null 2>&1; then
    curl --fail --location --silent --show-error --output "$output" "$url"
  elif command -v wget >/dev/null 2>&1; then
    wget -qO "$output" "$url"
  else
    die "curlまたはwgetが必要です。"
  fi
}

print_banner
step 1 4 "リリースを確認"
if [[ "$version" == "latest" ]]; then
  info "GitHubから最新バージョンを取得しています..."
  release_json=$(download "${api_base_url%/}/releases/latest" -)
  version=$(printf '%s\n' "$release_json" | sed -n 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p')
  if [[ -z "$version" ]]; then
    die "最新リリースのバージョンを取得できませんでした。"
  fi
fi

if [[ ! "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$ ]]; then
  die "無効なバージョンです: $version"
fi
success "Growse ${version}"

step 2 4 "実行環境を判定"
case "$(uname -s)" in
  Linux*)
    platform=linux
    extension=tar.gz
    executable=growse
    ;;
  Darwin*)
    platform=macos
    extension=tar.gz
    executable=growse
    ;;
  MINGW* | MSYS* | CYGWIN*)
    platform=windows
    extension=zip
    executable=growse.exe
    ;;
  *)
    die "未対応のOSです: $(uname -s)"
    ;;
esac

case "$(uname -m)" in
  x86_64 | amd64)
    arch=amd64
    ;;
  arm64 | aarch64)
    arch=arm64
    ;;
  *)
    die "未対応のCPUアーキテクチャです: $(uname -m)"
    ;;
esac

if [[ "$platform" != "macos" && "$arch" == "arm64" ]]; then
  die "${platform} arm64向けのリリースはまだありません。"
fi
success "${platform}/${arch}"

archive="growse_${version}_${platform}_${arch}.${extension}"
checksum="${archive}.sha256"
release_url="${release_base_url%/}/${version}"
work_dir=$(mktemp -d)
trap 'rm -rf "$work_dir"' EXIT

step 3 4 "パッケージを取得して検証"
info "${archive}をダウンロードしています..."
download "${release_url}/${archive}" "${work_dir}/${archive}"
download "${release_url}/${checksum}" "${work_dir}/${checksum}"

expected_hash=$(awk '{print $1; exit}' "${work_dir}/${checksum}")
if command -v sha256sum >/dev/null 2>&1; then
  actual_hash=$(sha256sum "${work_dir}/${archive}" | awk '{print $1}')
elif command -v shasum >/dev/null 2>&1; then
  actual_hash=$(shasum -a 256 "${work_dir}/${archive}" | awk '{print $1}')
elif command -v openssl >/dev/null 2>&1; then
  actual_hash=$(openssl dgst -sha256 "${work_dir}/${archive}" | awk '{print $NF}')
else
  die "SHA-256を検証できるコマンドがありません。"
fi

if [[ "$actual_hash" != "$expected_hash" ]]; then
  die "SHA-256チェックサムが一致しません。"
fi
success "SHA-256チェックサムを確認"

package_dir="${work_dir}/package"
mkdir -p "$package_dir"
if [[ "$extension" == "zip" ]]; then
  if ! command -v unzip >/dev/null 2>&1; then
    die "Windows版の展開にはunzipが必要です。"
  fi
  unzip -q "${work_dir}/${archive}" -d "$package_dir"
else
  tar -xzf "${work_dir}/${archive}" -C "$package_dir"
fi

case "$platform" in
  linux)
    desktop_source="${package_dir}/share/applications/${app_id}.desktop"
    icon_source="${package_dir}/share/icons/hicolor/512x512/apps/${app_id}.png"
    if [[ ! -f "$desktop_source" || ! -f "$icon_source" ]]; then
      die "Linux Desktop統合AssetがRelease Archiveにありません。"
    fi
    if [[ "$data_home" != /* ]]; then
      die "GROWSE_DATA_HOMEまたはXDG_DATA_HOMEには絶対Pathを指定してください: $data_home"
    fi
    ;;
  macos)
    application_source="${package_dir}/Growse.app"
    if [[ ! -x "${application_source}/Contents/MacOS/growse" ||
          ! -f "${application_source}/Contents/Info.plist" ||
          ! -f "${application_source}/Contents/Resources/Growse.icns" ]]; then
      die "macOS Application BundleがRelease Archiveにありません。"
    fi
    if [[ "$applications_dir" != /* ]]; then
      die "GROWSE_APPLICATIONS_DIRには絶対Pathを指定してください: $applications_dir"
    fi
    ;;
  windows)
    windows_icon_source="${package_dir}/share/windows/Growse.ico"
    shortcut_script_source="${package_dir}/share/windows/install-shortcut.ps1"
    if [[ ! -f "${package_dir}/growse.exe" ||
          ! -f "$windows_icon_source" ||
          ! -f "$shortcut_script_source" ]]; then
      die "Windows Application統合AssetがRelease Archiveにありません。"
    fi
    if [[ "$data_home" != /* ]]; then
      die "GROWSE_DATA_HOMEにはGit Bash上の絶対Pathを指定してください: $data_home"
    fi
    if ! command -v powershell.exe >/dev/null 2>&1 || ! command -v cygpath >/dev/null 2>&1; then
      die "Windows Start Menu登録にはpowershell.exeとcygpathが必要です。"
    fi
    ;;
esac

mkdir -p "$install_dir"
install_dir=$(cd "$install_dir" && pwd -P)

step 4 4 "Growseをインストール"
case "$platform" in
  linux)
    install -m 0755 "${package_dir}/growse" "${install_dir}/growse"
    desktop_dir="${data_home}/applications"
    icon_dir="${data_home}/icons/hicolor/512x512/apps"
    mkdir -p "$desktop_dir" "$icon_dir"

    desktop_path="${desktop_dir}/${app_id}.desktop"
    desktop_temp="${work_dir}/${app_id}.desktop"
    executable_path="${install_dir}/growse"
    executable_path=${executable_path//\\/\\\\}
    executable_path=${executable_path//\`/\\\`}
    executable_path=${executable_path//\$/\\\$}
    executable_path=${executable_path//\"/\\\"}
    while IFS= read -r line || [[ -n "$line" ]]; do
      if [[ "$line" == "Exec=growse" ]]; then
        printf 'Exec="%s"\n' "$executable_path"
      else
        printf '%s\n' "$line"
      fi
    done < "$desktop_source" > "$desktop_temp"

    install -m 0644 "$desktop_temp" "$desktop_path"
    install -m 0644 "$icon_source" "${icon_dir}/${app_id}.png"
    if command -v update-desktop-database >/dev/null 2>&1; then
      if ! update-desktop-database "$desktop_dir" >/dev/null; then
        warn "Desktop Databaseを更新できませんでした。再ログイン後に反映されます。"
      fi
    fi
    success "Desktop Applicationを登録"
    info "$desktop_path"
    ;;
  macos)
    mkdir -p "$applications_dir"
    applications_dir=$(cd "$applications_dir" && pwd -P)
    application_path="${applications_dir}/Growse.app"
    application_temp="${applications_dir}/.Growse.app.install.$$"
    cp -R "$application_source" "$application_temp"
    if [[ -e "$application_path" ]]; then
      if [[ ! -d "$application_path" || "$application_path" != */Growse.app ]]; then
        die "既存のGrowse.appを安全に更新できません: $application_path"
      fi
      rm -rf "$application_path"
    fi
    mv "$application_temp" "$application_path"
    ln -sfn "${application_path}/Contents/MacOS/growse" "${install_dir}/growse"
    success "macOS Applicationを配置"
    info "$application_path"
    ;;
  windows)
    install -m 0755 "${package_dir}/growse.exe" "${install_dir}/growse.exe"
    windows_asset_dir="${data_home}/growse"
    mkdir -p "$windows_asset_dir"
    windows_icon_path="${windows_asset_dir}/Growse.ico"
    install -m 0644 "$windows_icon_source" "$windows_icon_path"

    target_windows=$(cygpath -w "${install_dir}/growse.exe")
    icon_windows=$(cygpath -w "$windows_icon_path")
    script_windows=$(cygpath -w "$shortcut_script_source")
    powershell_arguments=(
      -NoProfile
      -NonInteractive
      -ExecutionPolicy Bypass
      -File "$script_windows"
      -TargetPath "$target_windows"
      -IconPath "$icon_windows"
    )
    if [[ -n "${GROWSE_WINDOWS_PROGRAMS_DIR:-}" ]]; then
      programs_directory="$GROWSE_WINDOWS_PROGRAMS_DIR"
      if [[ "$programs_directory" == /* ]]; then
        programs_directory=$(cygpath -w "$programs_directory")
      fi
      powershell_arguments+=( -ProgramsDirectory "$programs_directory" )
    fi
    shortcut_path=$(powershell.exe "${powershell_arguments[@]}" | tr -d '\r')
    success "Windows Start Menuへ登録"
    info "$shortcut_path"
    ;;
esac

success "Growse ${version}のインストールが完了しました"
printf '\n%b  Command%b  %s\n' "$color_bold" "$color_reset" "${install_dir}/${executable}"
if [[ ":${PATH}:" != *":${install_dir}:"* ]]; then
  warn "${install_dir}をPATHへ追加してください。"
fi
printf '\n'
