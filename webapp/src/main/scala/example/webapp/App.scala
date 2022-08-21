package example.webapp

import outwatch._
import outwatch.dsl._
import funstack.web.Fun

object App {

  // For styling, we use tailwindcss and daisyui:
  // - https://tailwindcss.com/ - basic styles like p-5, space-x-2, mb-auto, ...
  // - https://daisyui.com/ - based on tailwindcss with components like btn, navbar, footer, ...

  val layout = div(
    cls := "flex flex-col h-screen",
    pageHeader,
    pageBody,
    pageFooter,
  )

  def pageHeader =
    header(
      div(
        pageLink("Home", Page.Home),
        pageLink("API", Page.Api)(cls := "nav-api"),
        cls := "space-x-2",
      ),
      div(
        authControls,
        cls := "ml-auto",
      ),
      cls := "navbar shadow-lg",
    )

  def authControls =
    Fun.auth.currentUser.map {
      case Some(user) => a(s"Logout (${user.info.email})", href := Fun.auth.logoutUrl, cls := "btn btn-primary", cls := "logout-button")
      case None       => a("Login", href := Fun.auth.loginUrl, cls := "btn btn-primary", cls := "login-button")
    }

  def pageLink(name: String, page: Page): VNode = {
    val styling = Page.current.map {
      case `page` => cls := "btn-neutral"
      case _      => cls := "btn-ghost"
    }

    a(cls := "btn", name, page.href, styling)
  }

  def pageBody = div(
    cls := "p-10 mb-auto",
    // client-side router depending on the path in the address bar
    Page.current.map {
      case Page.Home =>
        div(
          cls := "text-bold",
          "Welcome!",
        )
      case Page.Api  =>
        div(
          cls := "space-y-4",
          Components.httpApi,
          Components.httpRpcApi,
          Components.websocketRpcApi,
          Components.websocketEvents,
        )
    },
  )

  def pageFooter =
    footer(
      cls := "p-5 footer bg-base-200 text-base-content footer-center",
      div(
        cls := "flex flex-row space-x-4",
        a(cls := "link link-hover", href := "#", "About us"),
        a(cls := "link link-hover", href := "#", "Contact"),
      ),
    )

}
