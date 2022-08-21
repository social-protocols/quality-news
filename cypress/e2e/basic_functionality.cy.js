describe("Basic functionality", function () {
  beforeEach(() => {
    cy.visit("http://localhost:12345");
  });

  it("HTTP API", function () {
    cy.get('.nav-api').click();
    cy.get('.tapir-result').should("have.text", "Right(List(Book(Programming in Scala)))");
  });

  it("HTTP RPC API", function () {
    cy.get('.nav-api').click();
    cy.get('.http-rpc-number-to-string').should("have.text", "3");
  });

  it("Websocket RPC API", function () {
    cy.get('.nav-api').click();
    cy.get('.websocket-rpc-number-to-string').should("have.text", "3");
  });

  it("Websocket Events", function () {
    cy.get('.nav-api').click();
    cy.get('.websocket-event-list').should("have.text", "");
    cy.get('.websocket-rpc-new-random-number-button').click();
    cy.get('.websocket-event-list').should("have.text", "Api Request by None!");
  });

  it("Auth", function () {
    cy.get('.login-button').should("have.text", "Login");
    cy.get('.login-button').click();

    // login screen
    cy.get('[type="text"]').clear().type("testuser");
    cy.get('[type="password"]').clear().type("testpw");
    cy.get(".login").click();

    cy.get('.logout-button').should("have.text", "Logout (testuser@localhost)");

    cy.get('.logout-button').click();
    cy.contains('Yes, sign me out').click();
    cy.get('.login-button').should("have.text", "Login");
  });
});
