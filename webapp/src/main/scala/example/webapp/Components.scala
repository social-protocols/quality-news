package example.webapp

import example.api.Api
import colibri.Subject
import outwatch.VModifier
import outwatch.dsl._
import funstack.web.tapir
import cats.effect.IO

object Components {
  import example.api.HttpApi

  def renderApi(api: Api[IO]): VModifier = {
    val currentRandomNumber = Subject.behavior[Option[Int]](None)

    VModifier(
      div(
        // example of rendering an async call directly
        // https://outwatch.github.io/docs/readme.html#rendering-futures
        // https://outwatch.github.io/docs/readme.html#rendering-async-effects
        b("Number: "),
        api.numberToString(3),
      ),
      div(
        // example of dynamic content with EmitterBuilder (onClick), IO (asEffect), and Subject/Observable/Observer (currentRandomNumber)
        // https://outwatch.github.io/docs/readme.html#dynamic-content
        b("Call: "),
        button(
          cls := "btn btn-primary btn-sm",
          "New Random Number",
          onClick.asEffect(api.getRandomNumber).map(Some.apply) --> currentRandomNumber,
        ),
        currentRandomNumber,
      ),
    )
  }

  val httpApi = div(
    h2("Http Api", cls := "text-xl"),
    div(
      // openapi with tapir
      b("My books: "),
      span(
        cls := "tapir-result",
        tapir.Fun.http
          .client(HttpApi.booksListing)((HttpApi.BooksFromYear("drama", 2011), 10))
          .map(_.toString),
      ),
    ),
  )

  def httpRpcApi = {
    val currentRandomNumber = Subject.behavior[Option[Int]](None)

    div(
      h2("Http Rpc Api", cls := "text-xl"),
      div(
        // example of rendering an async call directly
        // https://outwatch.github.io/docs/readme.html#rendering-futures
        // https://outwatch.github.io/docs/readme.html#rendering-async-effects
        b("Number to string via api call: "),
        span(HttpClient.api.numberToString(3), cls := "http-rpc-number-to-string"),
      ),
      div(
        // example of dynamic content with EmitterBuilder (onClick), IO (asEffect), and Subject/Observable/Observer (currentRandomNumber)
        // https://outwatch.github.io/docs/readme.html#dynamic-content
        div(
          b("Current random number: "),
          span(currentRandomNumber),
        ),
        button(
          "Get New Random Number from API",
          onClick.asEffect(HttpClient.api.getRandomNumber).map(Some.apply) --> currentRandomNumber,
          cls := "btn btn-primary btn-sm",
        ),
      ),
    )
  }

  def websocketRpcApi = {
    val currentRandomNumber = Subject.behavior[Option[Int]](None)

    div(
      h2("Websocket Rpc Api", cls := "text-xl"),
      div(
        // example of rendering an async call directly
        // https://outwatch.github.io/docs/readme.html#rendering-futures
        // https://outwatch.github.io/docs/readme.html#rendering-async-effects
        b("Number to string via api call: "),
        span(WsClient.api.numberToString(3), cls := "websocket-rpc-number-to-string"),
      ),
      div(
        // example of dynamic content with EmitterBuilder (onClick), IO (asEffect), and Subject/Observable/Observer (currentRandomNumber)
        // https://outwatch.github.io/docs/readme.html#dynamic-content
        div(
          b("Current random number: "),
          span(currentRandomNumber),
        ),
        button(
          "Get New Random Number from API",
          onClick.asEffect(WsClient.api.getRandomNumber).map(Some.apply) --> currentRandomNumber,
          cls := "btn btn-primary btn-sm",
          cls := "websocket-rpc-new-random-number-button",
        ),
      ),
    )
  }

  def websocketEvents =
    div(
      h2("Websocket Events", cls := "text-xl"),
      div(
        // incoming events from the websocket
        div("(press random number button)", cls := "text-gray-500"),
        div(
          WsClient.streamsApi.logs.map(div(_)).scanToList,
          cls                                   := "websocket-event-list",
        ),
      ),
    )

}
