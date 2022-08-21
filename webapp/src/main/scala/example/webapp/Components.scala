package example.webapp

import example.api.Api
import colibri.Subject
import outwatch.VModifier
import outwatch.dsl._
import funstack.web.tapir
import cats.effect.IO

object Components {

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

}
