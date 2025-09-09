#!/usr/bin/env bash

set -e

# ref:
# 1. https://github.com/kubernetes/code-generator
# 2. https://www.openshift.com/blog/kubernetes-deep-dive-code-generation-customresources
# 3. https://github.com/kubernetes/community/blob/master/contributors/devel/sig-api-machinery/generating-clientset.md

declare -A GROUP_VERSIONS=(
    [kubeflow]="v1"
    [acp]="v1alpha1"
    [cci]="v1alpha1"
    [eks]="v1alpha1"
    [management]="v1alpha1"
)

declare -A CRD_GEN_GROUP_VERSIONS=(
    [kubeflow]="v1"
    [acp]="v1alpha1"
    [cci]="v1alpha1"
    [eks]="v1alpha1"
    [management]="v1alpha1"
)

declare -A GROUP_GENERATORS=(
    [kubeflow]="client,deepcopy,informer,lister"
    [acp]="client,deepcopy,informer,lister"
    [cci]="client,deepcopy,informer,lister"
    [eks]="client,deepcopy,informer,lister"
    [management]="client,deepcopy,informer,lister"
)


################################################################################
# ********************* The Important Variable Definations *********************
################################################################################
declare -r CONTROLLER_GEN_VERSION="v0.6.2"
declare -r CODE_GENERATOR_VERSION="v0.21.4"
declare -r ROOT_DIR="$(cd $(dirname ${BASH_SOURCE[0]}) && pwd)"
declare -r BOILERPLATE_FILE="${ROOT_DIR}/hack/boilerplate.go.txt"

declare -r MODULE="$(awk '/module/{print $2}' ${ROOT_DIR}/go.mod)"
declare -r APIS_DIR="${MODULE}/pkg/apis"
declare -r CLIENT_DIR="${MODULE}/pkg/client"
declare -r CRD_DIR="${MODULE}/crds"

declare -r CLIENTSET_PKG_NAME="clientset"
declare -r CLIENTSET_NAME="versioned"
declare -r INFORMERS_NAME="informers"
declare -r LISTERS_NAME="listers"

declare -r DEEPCOPY_NAME="zz_generated.deepcopy"
declare -r DEFAULTER_NAME="zz_generated.defaulter"

################################################################################
# ********************** The Library Function Definations **********************
################################################################################

function LOG() {
    local level="debug"
    local texts="${*}"

    if [[ "${1,,}" =~ ^(info|debug|warn|error)$ ]]; then
        level="${1}"
        texts="${*:2}"
    fi

    case "${level}" in
    info) color_code="\e[0;32m" ;;
    debug) color_code="\e[0;34m" ;;
    warn) color_code="\e[0;33m" ;;
    error) color_code="\e[0;31m" ;;
    esac

    echo -e "${color_code}$(date +'%Y-%m-%d %H:%M:%S') [${level^^}] ${texts}\e[0m"
}

function gen_packages() {
    local -a packages

    for package in "${@}"; do
        packages+=("${APIS_DIR}/${package}")
    done

    tr ' ' ',' <<<"${packages[*]}"
}

function gen_informations() {
    local generator="${1}"
    LOG info "Generating the $(printf '%-9s' ${generator}) codes for the packages:" \
        "$(eval -- echo \${${generator}_pkgs[@]})"
}

function gen_crd_yaml() {
    local group="${1}"
    local version="${2}"

    local package_dir="${ROOT_DIR}/pkg/apis/${group}/${version}"
    local gvdir="${ROOT_DIR}/crds/${group}/${version}"
    local temp_dir="$(mktemp -d)"

    LOG info "Generating the \e[1;32mcrd\e[0;32m manifest file for" \
        "the package: \e[4;32m${group}/${version}"
    controller-gen +crd:crdVersions=v1 \
        +crd:generateEmbeddedObjectMeta=true \
        +paths="${package_dir}" \
        +output:dir="${temp_dir}"

    # skuinfo复数形式与inform不一致，不重新生成skuinfo的crd
    if [ "$group" = "acp" ]; then
        cp -f -r "${gvdir}/skuinfoes/crd.yaml" ${temp_dir}/acp.lepton.sensetime.com_skuinfoes.yaml
    fi

    for file in $(ls ${temp_dir}); do
        resource=$(echo ${file%.*} | awk -F_ '{print $NF}')
        mkdir -p ${gvdir}/${resource}
        cp -f -r ${temp_dir}/*_${resource}.yaml "${gvdir}/${resource}/crd.yaml"
    done
    rm -rf "${temp_dir}"
}

#####################################################################################
# *********************** Validating The Required Environment ***********************
#####################################################################################

if ! which go &>/dev/null; then
    LOG error "The go compiler is NOT installed locally!"
    exit 1
fi

IFS=. read cur_go_{major,minor}_version <<<$(go version | grep -oP '(?<=go)[\d.]+(?=\.\d+)')
IFS=. read go_mod_{major,minor}_version <<<$(awk '/^go/{print $2}' ${ROOT_DIR}/go.mod | grep -oP '\d+\.\d+')

if (((cur_go_major_version * 100 + cur_go_minor_version) < (go_mod_major_version * 100 + go_mod_minor_version))); then
    LOG error "Current go version $(go version | grep -oP '(?<=go)[\d.]+') is less than the" \
        "required go version $(awk '/^go/{print $2}' ${ROOT_DIR}/go.mod) in go.mod"
    exit 2
fi

gobin="$(go env GOBIN)"
export PATH="${gobin:-$(go env GOPATH)/bin}:${PATH}"

cd "${ROOT_DIR}"
mkdir -p "$(dirname ${MODULE})"
trap "rm -rf $(dirname ${MODULE})" EXIT
ln -sf "${ROOT_DIR}" "${MODULE}"

for genbin in {client,deepcopy,defaulter,informer,lister}-gen; do
    which "${genbin}" &>/dev/null && continue
    LOG debug "Downloading and compiling the ${genbin} binary file ..."
    go install "k8s.io/code-generator/cmd/${genbin}@${CODE_GENERATOR_VERSION}"
done

if ! which controller-gen &>/dev/null ||
    [[ "$(controller-gen --version | grep -oP 'v[0-9.]+')" != "${CONTROLLER_GEN_VERSION}" ]]; then
    LOG debug "Downloading and compiling the controller-gen-${CONTROLLER_GEN_VERSION} binary file ..."
    (
        set -e
        temp_dir="$(mktemp -d)"
        cd "${temp_dir}"
        go mod init tmp &>/dev/null
        go install "sigs.k8s.io/controller-tools/cmd/controller-gen@${CONTROLLER_GEN_VERSION}"
        rm -rf "${temp_dir}"
    )
fi

for group in ${!CRD_GEN_GROUP_VERSIONS[@]}; do
    for version in ${CRD_GEN_GROUP_VERSIONS[${group}]//,/ }; do
        gen_crd_yaml ${group} ${version}
    done
done

declare -a {deepcopy,defaulter,client,lister,informer}_pkgs

for group in ${!GROUP_VERSIONS[@]}; do
    for version in ${GROUP_VERSIONS[${group}]//,/ }; do
        for generator in ${GROUP_GENERATORS[${group}]//,/ }; do
            eval -- "${generator}_pkgs+=(${group}/${version})"
        done
    done
done

find "${ROOT_DIR}" -name "${DEEPCOPY_NAME}.go" -o -name "${DEFAULTER_NAME}.go" | xargs rm -f
rm -rf "${CLIENT_DIR}"

gen_informations deepcopy
deepcopy-gen --bounding-dirs "${APIS_DIR}" \
    --go-header-file "${BOILERPLATE_FILE}" \
    --input-dirs "$(gen_packages ${deepcopy_pkgs[@]})" \
    --output-base "${ROOT_DIR}" \
    --output-file-base "${DEEPCOPY_NAME}"

gen_informations defaulter
defaulter-gen --go-header-file "${BOILERPLATE_FILE}" \
    --input-dirs "$(gen_packages ${defaulter_pkgs[@]})" \
    --output-base "${ROOT_DIR}" \
    --output-file-base "${DEFAULTER_NAME}"

gen_informations client
client-gen --clientset-name "${CLIENTSET_NAME}" \
    --go-header-file "${BOILERPLATE_FILE}" \
    --input "$(gen_packages ${client_pkgs[@]})" \
    --input-base "" \
    --output-base "${ROOT_DIR}" \
    --output-package "${CLIENT_DIR}/${CLIENTSET_PKG_NAME}"

gen_informations lister
lister-gen --go-header-file "${BOILERPLATE_FILE}" \
    --input-dirs "$(gen_packages ${lister_pkgs[@]})" \
    --output-base "${ROOT_DIR}" \
    --output-package "${CLIENT_DIR}/${LISTERS_NAME}"

gen_informations informer
informer-gen --go-header-file "${BOILERPLATE_FILE}" \
    --input-dirs "$(gen_packages ${informer_pkgs[@]})" \
    --versioned-clientset-package "${CLIENT_DIR}/${CLIENTSET_PKG_NAME}/${CLIENTSET_NAME}" \
    --listers-package "${CLIENT_DIR}/${LISTERS_NAME}" \
    --output-base "${ROOT_DIR}" \
    --output-package "${CLIENT_DIR}/${INFORMERS_NAME}"

