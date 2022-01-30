export const logsPage = {
  logLinesNotContain: (lines: string) => cy.get('.pf-c-log-viewer__text', {timeout: 6000}).should('not.contain.text', lines),
  logWindowLoaded: () => cy.get('.pf-c-log-viewer__text', {timeout: 6000}).should('exist'),
  filterByUnit: (unitname: string) => {
    cy.get('#log-unit').clear();
    cy.get('#log-unit').type(unitname).type('{enter}');
  },
  selectLogComponent: (componentname: string) => {
    cy.get('button.pf-c-select__toggle').click();
    cy.get('.pf-c-select__menu-item').contains(componentname).click();
  },
  selectLogFile: (logname: string) => {
    cy.get('button.pf-c-select__toggle').last().click();
    cy.get('.pf-c-select__menu-item').contains(logname).click();
  }
}