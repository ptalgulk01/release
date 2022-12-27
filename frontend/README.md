# OpenShift Console tests
openshift web tests relies on upstream [openshift/console](https://github.com/openshift/console/tree/master) which provides fundamental configurations, views that we can reuse in openshift-tests-private web tests

## Prerequisite
1. [node.js](https://nodejs.org/) >= 14 & [yarn](https://yarnpkg.com/en/docs/install) >= 1.20
2. upstream [openshift/console](https://github.com/openshift/console/tree/master) should be cloned locally
3. upstream openshift/console dependencies need to be installed, for example we cloned openshift/console repo and save it to ~/reops
   - cd ~/reops/console/frontend
   - yarn install
4. link openshift/console in `frontend` folder and rename it as `upstream`
   - make sure you are in `frontend` folder of openshift-tests-private repo
   - ln -s ~/reops/console/frontend/packages/integration-tests-cypress upstream


**[Note] ALL following steps will run in `frontend` directory of openshift-tests-private repo**
## Install dependencies
all required dependencies are defined in `packege.json` in order to run Cypress tests, run `yarn install` so that dependencies will be installed in `node_modules` folder
```bash
$ yarn install
$ ls -ltr
node_modules/     -> dependencies will be installed at runtime here
```
## Directory structure
after dependencies are installed successfully and before we run actual tests, please confirm if we have correct structure as below, two new folders will be created after above
```bash
$ ls frontend
lrwxr-xr-x  upstream -> /xxx/console/frontend/packages/integration-tests-cypress
drwxr-xr-x  node_modules
````


### Export necessary variables
in order to run Cypress tests, we need to export some environment variables that Cypress can read then pass down to our tests, currently we have following environment variables defined and used.
```bash
export CYPRESS_BASE_URL=https://<console_route_spec_host>
export CYPRESS_LOGIN_IDP=kube:admin
export CYPRESS_LOGIN_USERNAME=testuser
export CYPRESS_LOGIN_PASSWORD=testpassword
export CYPRESS_KUBECONFIG_PATH=/path/to/kubeconfig
export CYPRESS_LOGIN_UP_PAIR=uiauto1:redhat(optional)
```
### Start Cypress
we can either open Cypress GUI(open) or run Cypress in headless mode(run)
```bash
npx cypress open
npx cypress run --env grep="Smoke"

```
