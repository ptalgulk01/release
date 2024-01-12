export const searchPage = {
  navToSearchPage: () => cy.visit('/search/all-namespaces'),
  chooseResourceType: (resource_type) => {
    cy.get('button[aria-label="Options menu"]').click();
    cy.get('input[type="search"]').clear().type(`${resource_type}`);
    cy.get('input[type="checkbox"]').first().click();
  },
  checkNoMachineResources: () => {
    searchPage.navToSearchPage();
    cy.get('button.pf-c-select__toggle').click();
    cy.get('[placeholder="Select Resource"]').type("machine");
    cy.contains('No results found');
  },
  clearAllFilters: () => {
    cy.byButtonText('Clear all filters').click();
  },
  searchMethodValues: (method, value) => {
    cy.get('button[id="toggle-id"]').click();
    cy.get(`button[name="${method}"]`).click();
    cy.get('input[id="search-filter-input"]').clear().type(`${value}`);
  },
  searchBy: (text) => {
    cy.get('input[data-test-id="item-filter"]').clear().type(`${text}`)
  },
}
