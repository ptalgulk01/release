import { guidedTour } from '../../upstream/views/guided-tour';
import { Pages } from '../../views/pages';
import { installedOperatorPage } from '../../views/operator-hub-page';

describe("Features on managed cluster such as ROSA/OSD", () => {
  before( () => {
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.login(
      Cypress.env("LOGIN_IDP"),
      Cypress.env("LOGIN_USERNAME"),
      Cypress.env("LOGIN_PASSWORD")
    );
    guidedTour.close();
  });

  after(() => {
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
  });

  it("(OCP-68381,yanpzhan,UI) Hide page-specific doc links for ROSA and OSD", {tags: ['e2e','@osd-ccs','@rosa','admin']}, function() {
    cy.isManagedCluster().then(value => {
      if(value == false){
        this.skip();
      }
    });
    cy.switchPerspective('Administrator');
    //hide update help link
    Pages.gotoClusterDetailspage();
    cy.get('a').should('not.contain', 'Learn more about');

    //hide project creation help link
    Pages.gotoProjectCreationPage();
    cy.get('a').should('not.contain', 'Learn more');

    //hide dc help link
    Pages.gotoDeploymentConfigList('default');
    cy.get('a').should('not.contain', 'Learn more about');

    //hide operators help link
    installedOperatorPage.goToWithNS('default');
    cy.get('a').should('not.contain', 'Understanding Operators');

    //hide networkpolicy help link
    cy.adminCLI(`oc create -f ./fixtures/testnp-OCP-68381.yaml -n default`);
    Pages.gotoOneNetworkPolicyDetails('default', 'testnp');
    cy.get('a').should('not.contain', 'NetwordPolicies documentation');
    cy.adminCLI(`oc delete networkpolicies testnp -n default`);

    //hide projec access help link
    Pages.gotoOneProjectAcessTab('openshift-console');
//    cy.get('a[data-quickstart-id="qs-nav-project"]').click();
//    cy.get('[data-test-id="horizontal-link-Project access"]').click();
    cy.get('a').should('not.contain', 'access control documentation');
    cy.switchPerspective('Administrator');
  });

  it("(OCP-68228,yanpzhan,UI) Update button is disabled on ROSA/OSD cluster", {tags: ['e2e','@osd-ccs','@rosa','admin']}, function() {
    cy.isManagedCluster().then(value => {
      if(value == false){
        this.skip();
      }
    });
    Pages.gotoClusterDetailspage();
    cy.get('button[data-test-id="current-channel-update-link"]').should('not.exist');
  })
})
