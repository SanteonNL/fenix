// Azure Blob Storage with SFTP access (ADLS Gen2)
// Subscription: Azure Sandbox
// Use: deploy an SFTP endpoint backed by Azure Storage for zbj-test source testing

@description('Azure region')
param location string = resourceGroup().location

@description('Storage account name (3-24 chars, lowercase alphanumeric)')
param storageAccountName string = 'zbjtest${uniqueString(resourceGroup().id)}'

@description('SFTP container (filesystem) name')
param containerName string = 'zbj-test'

@description('Local SFTP username')
param sftpUsername string = 'sftpuser'

@description('SSH public key for SFTP authentication (contents of id_rsa.pub)')
param sshPublicKey string

resource storageAccount 'Microsoft.Storage/storageAccounts@2023-04-01' = {
  name: storageAccountName
  location: location
  sku: {
    name: 'Standard_LRS'
  }
  kind: 'StorageV2'
  properties: {
    isHnsEnabled: true        // hierarchical namespace required for SFTP
    isSftpEnabled: true
    minimumTlsVersion: 'TLS1_2'
    allowBlobPublicAccess: false
    supportsHttpsTrafficOnly: true
  }
}

resource container 'Microsoft.Storage/storageAccounts/blobServices/containers@2023-04-01' = {
  name: '${storageAccount.name}/default/${containerName}'
  properties: {
    publicAccess: 'None'
  }
}

// Local user = SFTP identity; permission scoped to the container
resource localUser 'Microsoft.Storage/storageAccounts/localUsers@2023-04-01' = {
  name: '${storageAccount.name}/${sftpUsername}'
  properties: {
    hasSshKey: true
    hasSshPassword: false
    homeDirectory: containerName
    permissionScopes: [
      {
        service: 'blob'
        resourceName: containerName
        permissions: 'rcwdl'  // read, create, write, delete, list
      }
    ]
    sshAuthorizedKeys: [
      {
        description: 'zbj-test sftp key'
        key: sshPublicKey
      }
    ]
  }
  dependsOn: [container]
}

// SFTP connection string: <storageAccount>.blob.core.windows.net port 22
// Username format: <storageAccount>.<container>.<localUser>
output sftpHost string = '${storageAccount.name}.blob.core.windows.net'
output sftpUsername string = '${storageAccount.name}.${containerName}.${sftpUsername}'
output storageAccountId string = storageAccount.id
