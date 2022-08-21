package example.api

trait Api[F[_]] {
  def getFrontpage: F[List[Story]]
  def upvote(storyId: Int): F[Unit]
  def submit(): F[Unit]
}

case class Story(id: Int, title: String, url: String)
