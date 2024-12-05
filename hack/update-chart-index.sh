#!/usr/bin/env bash
# Copyright 2024 The Karmada Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -o errexit
set -o nounset
set -o pipefail

REPO_ROOT=$(dirname "${BASH_SOURCE[0]}")/..
cd ${REPO_ROOT}

# variable define
NEWBRANCH="helm-index-$(date +%s)"

get_latest_release() {
  curl --silent "https://api.github.com/repos/$1/releases/latest" |
  grep '"tag_name":' |
  sed -E 's/.*"([^"]+)".*/\1/'
}

# step1: get latest release tag
tag=$(get_latest_release "karmada-io/karmada")
if [ `grep -c "version: ${tag}" charts/index.yaml` -ge '2' ];then
    echo "latest tag already in helm index!"
    exit 0
fi

# step2: checkout a new branch
git checkout -b ${NEWBRANCH} origin/main

# step3: update index for karmada-chart
wget https://github.com/karmada-io/karmada/releases/download/${tag}/karmada-chart-${tag}.tgz -P charts/karmada/
helm repo index charts/karmada --url https://github.com/karmada-io/karmada/releases/download/${tag} --merge charts/index.yaml
mv charts/karmada/index.yaml charts/index.yaml

# step4: update index for karmada-operator-chart
wget https://github.com/karmada-io/karmada/releases/download/${tag}/karmada-operator-chart-${tag}.tgz -P charts/karmada-operator/
helm repo index charts/karmada-operator --url https://github.com/karmada-io/karmada/releases/download/${tag} --merge charts/index.yaml
mv charts/karmada-operator/index.yaml charts/index.yaml

# step5: commit the modification
git add charts/index.yaml
git commit -s -m "Bump upgrade helm chart index to ${tag}"
git push origin ${NEWBRANCH}

# step6: create pull request
prtext=$(
    cat <<EOF
**What type of PR is this?**

/kind cleanup

**What this PR does / why we need it**:

Bump upgrade helm chart index to ${tag}

**Which issue(s) this PR fixes**:

Fixes

**Does this PR introduce a user-facing change?**:
\`\`\`release-note
upgrade helm chart index to ${tag}.
\`\`\`
EOF
)
gh pr create --title "Bump upgrade helm chart index to ${tag}" --body "${prtext}" --base master --head ${NEWBRANCH}
