export const preferNotifications = {
  goToNotificationsTab: () => {
    cy.visit('/user-preferences/notifications');
    cy.wait(20000);
    cy.get('[data-test="tab notifications"]').invoke('attr', 'aria-selected').should('eq', 'true');
  },
  toggleNotifications: (action: string) => {
    cy.get("input[id='console.hideUserWorkloadNotifications']").then(($elem) => {
      const checkedstate = $elem.attr('data-checked-state');
      if(checkedstate === 'true'){
        cy.log('the "Hide user workload notifications" input is currently checked');
        if(action === 'hide'){
          cy.log('nothing to do since it already checked')
        } else if(action === 'enable') {
          cy.log('uncheck "Hide user workload notifications"');
          cy.contains('Hide user workload notifications').click();
        }
      } else if (checkedstate === 'false')
        cy.log('the "Hide user workload notifications" input is currently not checked');
        if(action === 'hide'){
          cy.log('check "Hide user workload notifications"');
          cy.contains('Hide user workload notifications').click();
        } else if(action === 'enable') {
          cy.log('nothing to do since it already un-checked')
        }
      })
  },
}
export const consoleTheme = {
  setDarkTheme: () => {
    cy.get('button[id="console.theme"]').click();
    cy.contains('button', 'Dark').click()
  },
  setLightTheme: () => {
    cy.get('button[id="console.theme"]').click();
    cy.contains('button', 'Light').click()
  },
  setSystemDefaultTheme: () => {
    cy.get('button[id="console.theme"]').click();
    cy.contains('button', 'System default').click()
  }
}

export const userPreferences = {
  navToGeneralUserPreferences: () => {
    cy.get('button[aria-label="User menu"]').click({force: true});
    cy.get('a').contains('User Preferences').click();
    cy.get('.co-user-preference-page-content__tab-content', {timeout: 20000}).should('be.visible');
  },
  checkExactMatchDisabledByDefault: () => {
    cy.get('input[id="console.enableExactSearch"]').should('have.attr', 'data-checked-state', 'false');
  },
  toggleExactMatch: (action: string) => {
    cy.get('input[id="console.enableExactSearch"]').as('enableExactMatchInput').then(($elem) => {
      const checkedstate = $elem.attr('data-checked-state');
      switch(checkedstate){
        case 'true':
          if(action === 'enable'){
            cy.log('exact match already enabled, nothing to do!');
          } else if( action === 'disable'){
            cy.log('exact match already enabled, disabling');
            cy.get('@enableExactMatchInput').click();
          }
        case 'false':
          if(action === 'enable') {
            cy.log('exact match currently disabled, enabling');
            cy.get('@enableExactMatchInput').click();
          } else if (action === 'disable') {
            cy.log('exact match currently disabled, nothing to do!');
          }
      }
    })
  }
}