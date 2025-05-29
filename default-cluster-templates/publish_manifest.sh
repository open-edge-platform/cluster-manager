set -eu -o pipefail


lpp_manifest_path="$(pwd)/default-cluster-templates/lpp-manifest.yaml"
echo "Using manifest path: $lpp_manifest_path"
if [ ! -f "${lpp_manifest_path}" ]; then
  echo "Error: Manifest file '$(lpp_manifest_path)' not found.";
  exit 1;
fi
RAW_CONTENT=$(< "$lpp_manifest_path")

declare -a templates=("$@")
lpp_cluster_config_manifest_path="/var/lib/rancher/rke2/server/manifests/local-path-provisioner.yaml"

for template_name in "${templates[@]}"; do
  template_file="$(pwd)/default-cluster-templates/${template_name}.json"
  if [ ! -f "${template_file}" ]; then
    echo "Error: Template file '$template_file' not found.";
    exit 1;
  fi

  echo "Processing template: $template_file"
  # echo "Raw content: $RAW_CONTENT"
  # Escape the content using jq
  ESCAPED_MANIFEST=$(jq -Rn --arg content "$RAW_CONTENT" '
    $content
    # | gsub("\\\\"; "\\\\\\\\")               # \ -> \\\\ not needed, jq handles this
    | gsub("\""; "\\\"")                     # " -> \"
    | gsub("\\$VOL_DIR"; "\\$VOL_DIR")       # $VOL_DIR -> \\$VOL_DIR
    | gsub("\n"; "\\n")                      # \n -> \\n
  ')
  echo "Escaped manifest: $ESCAPED_MANIFEST"
  # Replace only the correct 'content' key where the placeholder exists
  jq --arg manifest "$ESCAPED_MANIFEST" '
    .clusterconfiguration.spec.template.spec.files |= map(
      if .path == "/var/lib/rancher/rke2/server/manifests/local-path-provisioner.yaml"
      then .content = ($manifest | fromjson)
      else .
      end

    )
  ' "$template_file" > "$template_file.tmp" && mv "$template_file.tmp" "$template_file"

done  


# BASELINE_TEMPLATE=$(pwd)/default-cluster-templates/baseline.json
# LPP_PLACEHOLDER=__LPP_MANIFEST_PLACEHOLDER__

# first and second iterations
# echo $LPP_MANIFEST_PATH
# test_var=$(jq -Rs '.' < $LPP_MANIFEST_PATH)

# Double escape the JSON string
# DOUBLE_ESCAPED=$(echo "$test_var" | sed 's/\\/\\\\/g; s/\n/\\n/g')
# QUOTE_ESCAPED=$(echo "$DOUBLE_ESCAPED" | sed 's/^"//; s/"$//')
# TRIPLE_ESCAPED=$(echo "$QUOTE_ESCAPED" | sed 's/"/\\\\\\\\\\\\\\"/g')
# # Verify the double-escaped content
# echo "quote-escaped content: $TRIPLE_ESCAPED"
# # echo "test var: $(test_var)"
# # sed -i.bak 's@__MANIFEST_PLACEHOLDER__@'"$(printf '%s\n' "$(test_var)")"'@' baseline.json
# # sed -i.bak 's@__MANIFEST_PLACEHOLDER__@'"$(printf '%s\n' "$test_var")"'@' $BASELINE_TEMPLATE

# awk -v placeholder="$LPP_PLACEHOLDER" -v replacement="$TRIPLE_ESCAPED" '{gsub(placeholder, replacement); print}' "$BASELINE_TEMPLATE" > "$BASELINE_TEMPLATE.tmp" && mv "$BASELINE_TEMPLATE.tmp" "$BASELINE_TEMPLATE"


# echo "test var end"

# second iteration
# CONTENT_STRIPPED=${test_var:1:-1}
# ESCAPED_FOR_JSON=$(echo "$CONTENT_STRIPPED" | sed -e 's/\\/\\\\\\\\/g' -e 's/"/\\\\\\"/g')
# ESCAPED_DOLLAR=$(echo "$ESCAPED_FOR_JSON" | sed  -e 's/\$VOL_DIR/\\\\\\\\\$VOL_DIR/g')
# awk -v placeholder="$LPP_PLACEHOLDER" -v replacement="$ESCAPED_DOLLAR" '{
#     gsub(placeholder, replacement);
#     print
# }' "$BASELINE_TEMPLATE" > "$BASELINE_TEMPLATE.tmp" && mv "$BASELINE_TEMPLATE.tmp" "$BASELINE_TEMPLATE"

# third iteration
# Read the manifest as raw text
# RAW_CONTENT=$(< "$LPP_MANIFEST_PATH")
# echo "Raw content: $RAW_CONTENT"
# # Escape the content using jq
# ESCAPED_MANIFEST=$(jq -Rn --arg content "$RAW_CONTENT" '
#   $content
#   # | gsub("\\\\"; "\\\\\\\\")               # \ -> \\\\ not needed, jq handles this
#   | gsub("\""; "\\\"")                     # " -> \"
#   | gsub("\\$VOL_DIR"; "\\$VOL_DIR")       # $VOL_DIR -> \\$VOL_DIR
#   | gsub("\n"; "\\n")                      # \n -> \\n
# ')
# echo "Escaped manifest: $ESCAPED_MANIFEST"
# # Replace only the correct 'content' key where the placeholder exists
# jq --arg manifest "$ESCAPED_MANIFEST" '
#   .clusterconfiguration.spec.template.spec.files |= map(
#     if .path == "/var/lib/rancher/rke2/server/manifests/local-path-provisioner.yaml"
#     then .content = ($manifest | fromjson)
#     else .
#     end

#   )
# ' "$BASELINE_TEMPLATE" > "$BASELINE_TEMPLATE.tmp" && mv "$BASELINE_TEMPLATE.tmp" "$BASELINE_TEMPLATE"
