set -eu -o pipefail


lpp_manifest_file="$(pwd)/default-cluster-templates/manifests/lpp-manifest.yaml"
lpp_manifest_path_value="/var/lib/rancher/rke2/server/manifests/local-path-provisioner.yaml"

echo "using manifest from: $lpp_manifest_file"
if [ ! -f "${lpp_manifest_file}" ]; then
  echo "Error: Manifest file '$(lpp_manifest_path)' not found.";
  exit 1;
fi
lpp_manifest_content=$(< "$lpp_manifest_file")
encoded_manifest=$(jq -Rn --arg content "$lpp_manifest_content" '
    $content
    # | gsub("\\\\"; "\\\\\\\\")               # \ -> \\\\ not needed, jq handles this
    | gsub("\""; "\\\"")                     # " -> \"
    | gsub("\\$VOL_DIR"; "\\$VOL_DIR")       # $VOL_DIR -> \\$VOL_DIR
    | gsub("\n"; "\\n")                      # \n -> \\n
  ')
echo "manifest encoded"

templates=("$@")

for template_name in "${templates[@]}"; do
  template_file="$(pwd)/default-cluster-templates/${template_name}.json"
  if [ ! -f "${template_file}" ]; then
    echo "Error: Template file '$template_file' not found.";
    exit 1;
  fi

  # adding file member for LPP manifest
  jq --arg content "$template_file" --arg manifest_path "$lpp_manifest_path_value" '
    .clusterconfiguration.spec.template.spec.files += (
    if any(.clusterconfiguration.spec.template.spec.files[]; .path == $manifest_path)
    then []
    else [
      {
        path: $manifest_path,
        content: ""
      }
    ]
    end
  )' "$template_file" > "$template_file.tmp" && mv "$template_file.tmp" "$template_file"
  echo "lpp manifest path and content added to template"

  echo "processing template: $template_file"
  # replace only the correct 'content' key where the placeholder exists
  jq --arg manifest "$encoded_manifest" --arg manifest_path "$lpp_manifest_path_value" '
    .clusterconfiguration.spec.template.spec.files |= map(
      if .path == $manifest_path
      then .content = ($manifest | fromjson)
      else .
      end

    )
  ' "$template_file" > "$template_file.tmp" && mv "$template_file.tmp" "$template_file"
  echo "template update"

done
