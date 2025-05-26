set -xeu -o pipefail

LPP_MANIFEST_PATH=$(pwd)/default-cluster-templates/lpp-manifest.yaml
BASELINE_TEMPLATE=$(pwd)/default-cluster-templates/baseline.json
LPP_PLACEHOLDER=__LPP_MANIFEST_PLACEHOLDER__

echo $LPP_MANIFEST_PATH
test_var=$(jq -Rs '.' < $LPP_MANIFEST_PATH)

# Double escape the JSON string
DOUBLE_ESCAPED=$(echo "$test_var" | sed 's/\\/\\\\/g; s/\n/\\n/g')
QUOTE_ESCAPED=$(echo "$DOUBLE_ESCAPED" | sed 's/^"//; s/"$//')
# Verify the double-escaped content
echo "quote-escaped content: $QUOTE_ESCAPED"
# echo "test var: $(test_var)"
# sed -i.bak 's@__MANIFEST_PLACEHOLDER__@'"$(printf '%s\n' "$(test_var)")"'@' baseline.json
# sed -i.bak 's@__MANIFEST_PLACEHOLDER__@'"$(printf '%s\n' "$test_var")"'@' $BASELINE_TEMPLATE

awk -v placeholder="$LPP_PLACEHOLDER" -v replacement="$QUOTE_ESCAPED" '{gsub(placeholder, replacement); print}' "$BASELINE_TEMPLATE" > "$BASELINE_TEMPLATE.tmp" && mv "$BASELINE_TEMPLATE.tmp" "$BASELINE_TEMPLATE"


echo "test var end"