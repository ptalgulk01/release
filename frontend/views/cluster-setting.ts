export const ClusterSettingPage = {
  goToClusterSettingConfiguration: () => cy.visit('/settings/cluster/globalconfig'),
  clickToClustSettingDetailTab: () => cy.get('[data-test-id="horizontal-link-Details"]').click(),
  checkUpstreamUrlDisabled: () => cy.get('button[data-test-id="cluster-version-upstream-server-url"]').should("have.attr", "aria-disabled").and("eq", "true"),
  checkAlertMsg: (msg) => {
    cy.get('h4.pf-c-alert__title').should('contain', `${msg}`);
  },
  checkChannelNotEditable: () => cy.get('button[data-test-id="current-channel-update-link"]').should('not.exist'),
  checkNoAutoscalerField: () => cy.get('dt').should('not.contain', 'Cluster autoscaler'),
  checkClusterVersionNotEditable: () => {
    cy.get('[data-test-id="version"]').click();
    ClusterSettingPage.checkUpstreamUrlDisabled();
    cy.get('[data-test="Labels-details-item__edit-button"]').should('not.exist');
    cy.get('[data-test="edit-annotations"]').should('not.exist');
    cy.get('[data-test-id="horizontal-link-public~YAML"]').click();
    cy.get('.yaml-editor').should('exist');
    cy.get('[id="save-changes"]').should('not.exist');
  },
  checkHiddenConfiguration: () => {
    cy.get('[data-test-id="breadcrumb-link-0"]').click();
    cy.get('.loading-box__loaded').should('exist');
    const configName = ['APIServer','Authentication','DNS','FeatureGate','Networking','OAuth','Proxy','Scheduler'];
    configName.forEach(function (name) {
      cy.get(`[href="/k8s/cluster/config.openshift.io~v1~${name}/cluster"]`).should('not.exist');
    })
  },
  editUpstreamConfig: () => {
    cy.get('[data-test-id="cluster-version-upstream-server-url"]').click();
    cy.get('[data-test="Custom update service.-radio-input"]').click();
    cy.get('[id="cluster-version-custom-upstream-server-url"]')
      .clear()
      .type('https://openshift-release.apps.ci.l2s4.p1.openshiftapps.com/graph');
    cy.get('[data-test="confirm-action"]').click();
    
  },
  configureChannel: () => {
    cy.get('[data-test-id="cluster-version"]').then(($version) => {
      const text = $version.text();
      const versionString = `stable-${text.split('.').slice(0, 2).join('.')}`
      cy.get('[data-test-id="current-channel-update-link"]').click();
      cy.get('.pf-c-form-control').clear().type(versionString);
    });
    cy.get('[data-test="confirm-action"]').click();
  },
}
