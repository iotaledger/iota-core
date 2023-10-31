eval "$NETWORK_ENVIRONMENT"

export ANSIBLE_STRATEGY=free
export ANSIBLE_PIPELINING=true
export ANSIBLE_PERSISTENT_CONTROL_PATH_DIR="/tmp/"

ARGS=("$@")
ansible-playbook -u root -i deploy/ansible/hosts/"${1:-feature.yml}" \
  --forks 20 --ssh-common-args "-o ControlMaster=auto -o ControlPersist=5m" \
  --extra-vars \
"customSnapshotUrl=$CUSTOM_SNAPSHOT_URL
defaultSnapshotUrl=$DEFAULT_SNAPSHOT_URL
iota_core_docker_image_repo=$IOTA_CORE_DOCKER_IMAGE_REPO
iota_core_docker_image_tag=$IOTA_CORE_DOCKER_IMAGE_TAG
wireguard_server_private_key=$WIREGUARD_SERVER_PRIVKEY
elkElasticUser=$ELASTIC_USER
elkElasticPassword=$ELASTIC_PASSWORD
grafanaAdminPassword=$GRAFANA_ADMIN_PASSWORD

NODE_01_VALIDATOR_ACCOUNTADDRESS=$NODE_01_VALIDATOR_ACCOUNTADDRESS
NODE_01_VALIDATOR_PRIVKEY=$NODE_01_VALIDATOR_PRIVKEY
NODE_01_P2PIDENTITY_PRIVKEY=$NODE_01_P2PIDENTITY_PRIVKEY

NODE_02_VALIDATOR_ACCOUNTADDRESS=$NODE_02_VALIDATOR_ACCOUNTADDRESS
NODE_02_VALIDATOR_PRIVKEY=$NODE_02_VALIDATOR_PRIVKEY
NODE_02_P2PIDENTITY_PRIVKEY=$NODE_02_P2PIDENTITY_PRIVKEY

NODE_03_VALIDATOR_ACCOUNTADDRESS=$NODE_03_VALIDATOR_ACCOUNTADDRESS
NODE_03_VALIDATOR_PRIVKEY=$NODE_03_VALIDATOR_PRIVKEY
NODE_03_P2PIDENTITY_PRIVKEY=$NODE_03_P2PIDENTITY_PRIVKEY

NODE_04_BLOCKISSUER_ACCOUNTADDRESS=$NODE_04_BLOCKISSUER_ACCOUNTADDRESS
NODE_04_BLOCKISSUER_PRIVKEY=$NODE_04_BLOCKISSUER_PRIVKEY
NODE_04_FAUCET_PRIVKEY=$NODE_04_FAUCET_PRIVKEY
NODE_04_P2PIDENTITY_PRIVKEY=$NODE_04_P2PIDENTITY_PRIVKEY

NODE_05_P2PIDENTITY_PRIVKEY=$NODE_05_P2PIDENTITY_PRIVKEY" \

  ${ARGS[@]:2} deploy/ansible/"${2:-deploy.yml}"
