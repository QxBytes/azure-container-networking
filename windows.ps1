
function azure-npm-image {
    $env:ACN_PACKAGE_PATH = "github.com/Azure/azure-container-networking"
    $env:NPM_AI_ID = "014c22bd-4107-459e-8475-67909e96edcb"
    $env:NPM_AI_PATH="$env:ACN_PACKAGE_PATH/npm.aiMetadata"

    if ($null -eq $env:VERSION) { $env:VERSION = $args[0] } 
    docker build `
        -f npm/Dockerfile.windows `
        -t acnpublic.azurecr.io/azure-npm:$env:VERSION `
        --build-arg VERSION=$env:VERSION `
        --build-arg NPM_AI_PATH=$env:NPM_AI_PATH `
        --build-arg NPM_AI_ID=$env:NPM_AI_ID `
        .
}