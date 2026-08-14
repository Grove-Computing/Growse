#!/usr/bin/env bash

set -euo pipefail

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
    echo "curlまたはwgetが必要です。" >&2
    exit 1
  fi
}

if [[ "$version" == "latest" ]]; then
  release_json=$(download "${api_base_url%/}/releases/latest" -)
  version=$(printf '%s\n' "$release_json" | sed -n 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p')
  if [[ -z "$version" ]]; then
    echo "最新リリースのバージョンを取得できませんでした。" >&2
    exit 1
  fi
fi

if [[ ! "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$ ]]; then
  echo "無効なバージョンです: $version" >&2
  exit 1
fi

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
    echo "未対応のOSです: $(uname -s)" >&2
    exit 1
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
    echo "未対応のCPUアーキテクチャです: $(uname -m)" >&2
    exit 1
    ;;
esac

if [[ "$platform" != "macos" && "$arch" == "arm64" ]]; then
  echo "${platform} arm64向けのリリースはまだありません。" >&2
  exit 1
fi

archive="growse_${version}_${platform}_${arch}.${extension}"
checksum="${archive}.sha256"
release_url="${release_base_url%/}/${version}"
work_dir=$(mktemp -d)
trap 'rm -rf "$work_dir"' EXIT

echo "Growse $version ($platform/$arch) をダウンロードしています..."
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
  echo "SHA-256を検証できるコマンドがありません。" >&2
  exit 1
fi

if [[ "$actual_hash" != "$expected_hash" ]]; then
  echo "SHA-256チェックサムが一致しません。" >&2
  exit 1
fi

package_dir="${work_dir}/package"
mkdir -p "$package_dir"
if [[ "$extension" == "zip" ]]; then
  if ! command -v unzip >/dev/null 2>&1; then
    echo "Windows版の展開にはunzipが必要です。" >&2
    exit 1
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
      echo "Linux Desktop統合AssetがRelease Archiveにありません。" >&2
      exit 1
    fi
    if [[ "$data_home" != /* ]]; then
      echo "GROWSE_DATA_HOMEまたはXDG_DATA_HOMEには絶対Pathを指定してください: $data_home" >&2
      exit 1
    fi
    ;;
  macos)
    application_source="${package_dir}/Growse.app"
    if [[ ! -x "${application_source}/Contents/MacOS/growse" ||
          ! -f "${application_source}/Contents/Info.plist" ||
          ! -f "${application_source}/Contents/Resources/Growse.icns" ]]; then
      echo "macOS Application BundleがRelease Archiveにありません。" >&2
      exit 1
    fi
    if [[ "$applications_dir" != /* ]]; then
      echo "GROWSE_APPLICATIONS_DIRには絶対Pathを指定してください: $applications_dir" >&2
      exit 1
    fi
    ;;
  windows)
    windows_icon_source="${package_dir}/share/windows/Growse.ico"
    shortcut_script_source="${package_dir}/share/windows/install-shortcut.ps1"
    if [[ ! -f "${package_dir}/growse.exe" ||
          ! -f "$windows_icon_source" ||
          ! -f "$shortcut_script_source" ]]; then
      echo "Windows Application統合AssetがRelease Archiveにありません。" >&2
      exit 1
    fi
    if [[ "$data_home" != /* ]]; then
      echo "GROWSE_DATA_HOMEにはGit Bash上の絶対Pathを指定してください: $data_home" >&2
      exit 1
    fi
    if ! command -v powershell.exe >/dev/null 2>&1 || ! command -v cygpath >/dev/null 2>&1; then
      echo "Windows Start Menu登録にはpowershell.exeとcygpathが必要です。" >&2
      exit 1
    fi
    ;;
esac

mkdir -p "$install_dir"
install_dir=$(cd "$install_dir" && pwd -P)

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
        echo "Desktop Databaseを更新できませんでした。再ログイン後に反映されます。" >&2
      fi
    fi
    echo "GrowseをDesktop Applicationとして ${desktop_path} に登録しました。"
    ;;
  macos)
    mkdir -p "$applications_dir"
    applications_dir=$(cd "$applications_dir" && pwd -P)
    application_path="${applications_dir}/Growse.app"
    application_temp="${applications_dir}/.Growse.app.install.$$"
    cp -R "$application_source" "$application_temp"
    if [[ -e "$application_path" ]]; then
      if [[ ! -d "$application_path" || "$application_path" != */Growse.app ]]; then
        echo "既存のGrowse.appを安全に更新できません: $application_path" >&2
        exit 1
      fi
      rm -rf "$application_path"
    fi
    mv "$application_temp" "$application_path"
    ln -sfn "${application_path}/Contents/MacOS/growse" "${install_dir}/growse"
    echo "GrowseをmacOS Applicationとして ${application_path} にインストールしました。"
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
    echo "GrowseをWindows Start Menuへ ${shortcut_path} として登録しました。"
    ;;
esac

echo "Growse commandを ${install_dir}/${executable} にインストールしました。"
if [[ ":${PATH}:" != *":${install_dir}:"* ]]; then
  echo "${install_dir} をPATHへ追加してください。"
fi
